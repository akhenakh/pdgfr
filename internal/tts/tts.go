// Package tts turns French text into 22,050 Hz audio with Brindille.
//
// It runs the published Alkove WebAssembly module (the engine, the text
// frontend and the synthesizer) in-process with wazero, and feeds it the
// Brindille networks, the pronunciation dictionary and the synthesizer
// constants. There is no ONNX runtime, no model server and no file on disk.
package tts

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/akhenak/pdgfr/internal/assets"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// SampleRate is Brindille's output rate.
const SampleRate = 22050

// DefaultChunk is how much audio Stream hands to the caller at a time.
const DefaultChunk = SampleRate / 5 // 200 ms

// DefaultGap is the silence inserted between two sentences. Brindille puts
// roughly this much between sentences when it reads a whole paragraph.
const DefaultGap = SampleRate * 15 / 100 // 150 ms

// ErrNoSpeech is returned when the text has nothing to speak.
var ErrNoSpeech = errors.New("no speakable text")

// Voice holds Brindille's voice controls.
//
// Pitch is in semitones, intonation is a factor around 1, treble and breath
// are gains in dB. DefaultVoice is the published voice.
type Voice struct {
	Pitch      float32
	Intonation float32
	Treble     float32
	Breath     float32
}

// DefaultVoice is Estelle as published: no transposition, normal intonation,
// flat treble and breath.
var DefaultVoice = Voice{Intonation: 1}

// Result is one synthesized sentence.
type Result struct {
	Audio    []float32
	Phonemes string
}

// Engine is a loaded text-to-speech engine. It is safe for concurrent use;
// calls are serialized because one WebAssembly module has one arena.
type Engine struct {
	rt     wazero.Runtime
	mod    api.Module
	handle uint32

	alloc  api.Function
	free   api.Function
	newTTS api.Function
	speak  api.Function
	audio  api.Function
	voice  api.Function
	tokens api.Function
	errFn  api.Function

	symbols []string

	// snapshot is the module's memory right after the model loads. Restoring
	// it before each sentence resets the synthesizer's noise state, so every
	// call yields the same audio for the same text.
	snapshot []byte

	mu sync.Mutex
}

// New loads the embedded WebAssembly module and the Brindille model. It takes
// a few tens of milliseconds and needs no external files.
func New() (*Engine, error) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	mod, err := rt.InstantiateWithConfig(ctx, assets.Wasm, wazero.NewModuleConfig().WithName("tts"))
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("pdgfr: load engine: %w", err)
	}
	e := &Engine{
		rt:     rt,
		mod:    mod,
		alloc:  mod.ExportedFunction("tts_alloc"),
		free:   mod.ExportedFunction("tts_free"),
		newTTS: mod.ExportedFunction("tts_new"),
		speak:  mod.ExportedFunction("tts_speak"),
		audio:  mod.ExportedFunction("tts_audio"),
		voice:  mod.ExportedFunction("tts_voice"),
		tokens: mod.ExportedFunction("tts_tokens"),
		errFn:  mod.ExportedFunction("tts_error"),
	}
	if e.alloc == nil || e.free == nil || e.newTTS == nil || e.speak == nil ||
		e.audio == nil || e.voice == nil || e.tokens == nil || e.errFn == nil {
		e.Close()
		return nil, errors.New("pdgfr: engine is missing an export")
	}

	var meta struct {
		SampleRate int      `json:"sr"`
		Symbols    []string `json:"symbols"`
	}
	if err := json.Unmarshal(assets.Meta, &meta); err != nil {
		e.Close()
		return nil, fmt.Errorf("pdgfr: read symbols: %w", err)
	}
	if meta.SampleRate != SampleRate || len(meta.Symbols) == 0 {
		e.Close()
		return nil, fmt.Errorf("pdgfr: unexpected model metadata (sample rate %d)", meta.SampleRate)
	}
	e.symbols = meta.Symbols

	if err := e.load(); err != nil {
		e.Close()
		return nil, err
	}
	if err := e.snapshotMemory(); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

// Close releases the WebAssembly module and its arena.
func (e *Engine) Close() error {
	if e == nil || e.rt == nil {
		return nil
	}
	return e.rt.Close(context.Background())
}

// load hands the four files to tts_new, which copies them into the module's
// memory; the temporary buffers are freed afterwards.
func (e *Engine) load() error {
	bufs := make([][2]uint32, 0, 4)
	args := make([]uint64, 0, 8)
	for _, b := range [][]byte{assets.Model, assets.Lex, assets.Dict, assets.Params} {
		p, n, err := e.put(b)
		if err != nil {
			return err
		}
		bufs = append(bufs, [2]uint32{p, n})
		args = append(args, uint64(p), uint64(n))
	}
	defer func() {
		for _, b := range bufs {
			e.free.Call(context.Background(), uint64(b[0]), uint64(b[1]))
		}
	}()

	r, err := e.newTTS.Call(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("pdgfr: load model: %w", err)
	}
	if h := uint32(r[0]); h != 0 {
		e.handle = h
		return nil
	}
	return errors.New("pdgfr: " + e.lastError())
}

// put copies b into the module's memory and returns its pointer and length.
func (e *Engine) put(b []byte) (uint32, uint32, error) {
	r, err := e.alloc.Call(context.Background(), uint64(len(b)))
	if err != nil {
		return 0, 0, fmt.Errorf("pdgfr: allocate %d bytes: %w", len(b), err)
	}
	p := uint32(r[0])
	if p == 0 || !e.mod.Memory().Write(p, b) {
		return 0, 0, fmt.Errorf("pdgfr: cannot write %d bytes into the engine", len(b))
	}
	return p, uint32(len(b)), nil
}

// snapshotMemory keeps a copy of the module's memory after loading.
func (e *Engine) snapshotMemory() error {
	mem := e.mod.Memory()
	b, ok := mem.Read(0, mem.Size())
	if !ok {
		return errors.New("pdgfr: cannot read the engine memory")
	}
	e.snapshot = append([]byte(nil), b...)
	return nil
}

// resetMemory restores the post-load memory. It clears the synthesizer's noise
// state and the voice controls, so a sentence is synthesized the same way
// whatever ran before it. Pages the module added by growing are zeroed.
func (e *Engine) resetMemory() {
	mem := e.mod.Memory()
	mem.Write(0, e.snapshot)
	if size := mem.Size(); size > uint32(len(e.snapshot)) {
		var zero [64 << 10]byte
		for off := uint32(len(e.snapshot)); off < size; {
			n := size - off
			if n > uint32(len(zero)) {
				n = uint32(len(zero))
			}
			mem.Write(off, zero[:n])
			off += n
		}
	}
}

// lastError reads the module's last error message.
func (e *Engine) lastError() string {
	const cap = 512
	r, err := e.alloc.Call(context.Background(), cap)
	if err != nil {
		return err.Error()
	}
	p := uint32(r[0])
	defer e.free.Call(context.Background(), uint64(p), cap)
	n, err := e.errFn.Call(context.Background(), uint64(p), cap)
	if err != nil {
		return err.Error()
	}
	b, ok := e.mod.Memory().Read(p, uint32(n[0]))
	if !ok {
		return "engine error"
	}
	return string(b)
}

// Speak synthesizes one sentence. Audio is mono float32 at SampleRate; the
// phoneme string is the sequence Brindille pronounced, with " | " for a
// silence.
//
// Speed is clamped to 0.5–2, the range the model was trained for. An empty or
// unpronounceable sentence returns ErrNoSpeech.
func (e *Engine) Speak(text string, speed float32, v Voice) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		return Result{}, ErrNoSpeech
	}
	if speed <= 0 {
		speed = 1
	}
	if speed < 0.5 {
		speed = 0.5
	} else if speed > 2 {
		speed = 2
	}
	if v.Intonation <= 0 {
		v.Intonation = 1
	}
	ctx := context.Background()
	e.resetMemory()
	e.voice.Call(ctx,
		uint64(e.handle),
		uint64(math.Float32bits(v.Pitch)),
		uint64(math.Float32bits(v.Intonation)),
		uint64(math.Float32bits(v.Treble)),
		uint64(math.Float32bits(v.Breath)),
	)

	p, n, err := e.put([]byte(text))
	if err != nil {
		return Result{}, err
	}
	r, err := e.speak.Call(ctx, uint64(e.handle), uint64(p), uint64(n), uint64(math.Float32bits(speed)))
	e.free.Call(ctx, uint64(p), uint64(n))
	if err != nil {
		return Result{}, fmt.Errorf("pdgfr: synthesize: %w", err)
	}
	count := uint32(r[0])
	if count == 0 {
		if msg := e.lastError(); msg != "" && msg != "engine error" {
			return Result{}, fmt.Errorf("pdgfr: %s", msg)
		}
		return Result{}, ErrNoSpeech
	}

	ar, err := e.audio.Call(ctx, uint64(e.handle))
	if err != nil {
		return Result{}, fmt.Errorf("pdgfr: read audio: %w", err)
	}
	raw, ok := e.mod.Memory().Read(uint32(ar[0]), count*4)
	if !ok {
		return Result{}, errors.New("pdgfr: audio is outside the engine memory")
	}
	audio := make([]float32, count)
	for i := range audio {
		audio[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return Result{Audio: audio, Phonemes: e.phonemes()}, nil
}

// phonemes reads the tokens of the last Speak. The caller holds e.mu.
func (e *Engine) phonemes() string {
	ctx := context.Background()
	r, err := e.alloc.Call(ctx, 4)
	if err != nil {
		return ""
	}
	np := uint32(r[0])
	defer e.free.Call(ctx, uint64(np), 4)

	p, err := e.tokens.Call(ctx, uint64(e.handle), uint64(np))
	if err != nil {
		return ""
	}
	nb, ok := e.mod.Memory().Read(np, 4)
	if !ok {
		return ""
	}
	n := binary.LittleEndian.Uint32(nb)
	ids, ok := e.mod.Memory().Read(uint32(p[0]), n*4)
	if !ok {
		return ""
	}
	var b strings.Builder
	for i := uint32(0); i < n; i++ {
		id := binary.LittleEndian.Uint32(ids[i*4:])
		if int(id) >= len(e.symbols) {
			continue
		}
		if s := e.symbols[id]; s == "<sil>" {
			b.WriteString(" | ")
		} else {
			b.WriteString(s)
		}
	}
	return collapseSilences(b.String())
}

// collapseSilences turns repeated silence markers into one.
func collapseSilences(s string) string {
	for strings.Contains(s, " |  | ") {
		s = strings.ReplaceAll(s, " |  | ", " | ")
	}
	return strings.TrimSpace(s)
}
