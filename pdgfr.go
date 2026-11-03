// Package pdgfr is a zero-install, pure-Go French text-to-speech engine.
//
// It turns text into 22,050 Hz mono float32 audio on the CPU, with no Python,
// ONNX Runtime, model server or GPU. Brindille's network, Lex, the
// pronunciation dictionary and the Alkove runtime are embedded in the binary:
// there is no file to install and nothing to download at run time.
//
//	e, err := pdgfr.New()
//	if err != nil { ... }
//	audio, err := e.Synthesize(context.Background(), "Bonjour à tous.")
//	if err != nil { ... }
//	f, _ := os.Create("bonjour.wav")
//	defer f.Close()
//	pdgfr.WriteWAV(f, audio)
//
// For playback or streaming, use Stream. An Engine is safe for concurrent use;
// pass WithSpeed and WithVoice for the rate and the timbre.
package pdgfr

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/akhenak/pdgfr/internal/tts"
	"github.com/akhenak/pdgfr/internal/wav"
)

// SampleRate is the output sample rate in Hz.
const SampleRate = tts.SampleRate

// ErrNoSpeech is returned when the text has nothing to speak.
var ErrNoSpeech = errors.New("pdgfr: no speakable text")

// Engine is a loaded French text-to-speech engine. It is safe for concurrent
// use.
type Engine struct {
	synth *tts.Engine
}

// New loads the embedded engine and model. It takes a few tens of milliseconds
// and needs no external files.
func New() (*Engine, error) {
	s, err := tts.New()
	if err != nil {
		return nil, err
	}
	return &Engine{synth: s}, nil
}

// Close releases the engine's memory.
func (e *Engine) Close() error {
	if e == nil || e.synth == nil {
		return nil
	}
	return e.synth.Close()
}

// Option configures one Synthesize or Stream call.
type Option func(*config)

type config struct {
	speed    float32
	voice    tts.Voice
	chunk    int
	gap      int
	phonemes func(sentence, phonemes string)
}

func newConfig(opts []Option) config {
	c := config{
		speed: 1,
		voice: tts.DefaultVoice,
		chunk: tts.DefaultChunk,
		gap:   tts.DefaultGap,
	}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.speed <= 0 {
		c.speed = 1
	}
	if c.chunk <= 0 {
		c.chunk = tts.DefaultChunk
	}
	if c.gap < 0 {
		c.gap = 0
	}
	return c
}

// WithSpeed sets the speaking rate: 2 speaks twice as fast, 0.5 half as fast.
// Brindille was trained for 0.5–2; values outside that range are clamped.
func WithSpeed(speed float32) Option {
	return func(c *config) {
		if speed > 0 {
			c.speed = speed
		}
	}
}

// WithVoice sets the voice controls. Pitch is in semitones, intonation is a
// factor around 1, treble and breath are gains in dB. Zero values other than
// intonation keep the published voice; see tts.DefaultVoice.
func WithVoice(pitch, intonation, treble, breath float32) Option {
	return func(c *config) {
		c.voice = tts.Voice{
			Pitch:      pitch,
			Intonation: intonation,
			Treble:     treble,
			Breath:     breath,
		}
		if c.voice.Intonation <= 0 {
			c.voice.Intonation = 1
		}
	}
}

// WithChunk sets the slice size Stream hands to the caller. The default is
// 200 ms. A zero or negative duration keeps the default.
func WithChunk(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.chunk = samples(d)
		}
	}
}

// WithGap sets the silence Stream inserts between two sentences. The default
// is 150 ms, roughly what Brindille puts between sentences in one call.
func WithGap(d time.Duration) Option {
	return func(c *config) {
		if d >= 0 {
			c.gap = samples(d)
		}
	}
}

func samples(d time.Duration) int {
	return int(d.Seconds()*SampleRate + 0.5)
}

// WithPhonemes registers a callback invoked with each sentence and the phoneme
// sequence Brindille pronounced for it, before that sentence's audio is
// yielded. It is meant for diagnostics; synthesis costs the same.
func WithPhonemes(fn func(sentence, phonemes string)) Option {
	return func(c *config) {
		c.phonemes = fn
	}
}

// Synthesize returns the complete waveform for text. Samples are mono float32
// at SampleRate, roughly in [-1, 1].
//
// The text is read sentence by sentence so the answer is identical to
// concatenating Stream's chunks: the sentences are separated by the gap set
// with WithGap.
func (e *Engine) Synthesize(ctx context.Context, text string, opts ...Option) ([]float32, error) {
	var out []float32
	err := e.Stream(ctx, text, func(chunk []float32) error {
		out = append(out, chunk...)
		return nil
	}, opts...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNoSpeech
	}
	return out, nil
}

// Stream synthesizes text and calls yield with each audio slice as soon as it
// is ready. Brindille synthesizes a whole sentence at a time, so the first
// slice arrives once the first sentence is done; later slices are WithChunk
// long (200 ms by default).
//
// Returning an error from yield stops synthesis and returns that error.
// Cancellation is checked between slices and between sentences; a sentence
// already being synthesized is not interrupted.
func (e *Engine) Stream(ctx context.Context, text string, yield func([]float32) error, opts ...Option) error {
	if yield == nil {
		return errors.New("pdgfr: nil yield function")
	}
	if strings.TrimSpace(text) == "" {
		return ErrNoSpeech
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg := newConfig(opts)
	sentences := tts.SplitSentences(text)
	if len(sentences) == 0 {
		return ErrNoSpeech
	}

	wrote := false
	for _, sentence := range sentences {
		if err := ctx.Err(); err != nil {
			return err
		}
		res, err := e.synth.Speak(sentence, cfg.speed, cfg.voice)
		if errors.Is(err, tts.ErrNoSpeech) {
			continue
		}
		if err != nil {
			return err
		}
		if len(res.Audio) == 0 {
			continue
		}
		if cfg.phonemes != nil {
			cfg.phonemes(sentence, res.Phonemes)
		}
		if wrote && cfg.gap > 0 {
			gap := make([]float32, cfg.gap)
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := yield(gap); err != nil {
				return err
			}
		}
		for off := 0; off < len(res.Audio); off += cfg.chunk {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := off + cfg.chunk
			if end > len(res.Audio) {
				end = len(res.Audio)
			}
			if err := yield(res.Audio[off:end]); err != nil {
				return err
			}
		}
		wrote = true
	}
	if !wrote {
		return ErrNoSpeech
	}
	return nil
}

// Phonemes returns the sequence Brindille pronounces for text, with " | " for
// a silence. It synthesizes the text (there is no cheaper path through the
// published module) and discards the audio.
func (e *Engine) Phonemes(ctx context.Context, text string, opts ...Option) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", ErrNoSpeech
	}
	cfg := newConfig(opts)
	var out []string
	for _, sentence := range tts.SplitSentences(text) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		res, err := e.synth.Speak(sentence, cfg.speed, cfg.voice)
		if errors.Is(err, tts.ErrNoSpeech) {
			continue
		}
		if err != nil {
			return "", err
		}
		if res.Phonemes != "" {
			out = append(out, res.Phonemes)
		}
	}
	if len(out) == 0 {
		return "", ErrNoSpeech
	}
	return strings.Join(out, " "), nil
}

// WriteWAV writes mono 16-bit PCM WAV at SampleRate.
func WriteWAV(w io.Writer, samples []float32) error {
	return wav.Write(w, samples, SampleRate)
}

// PCM converts float32 samples to little-endian signed 16-bit PCM, ready for
// an audio device or a raw stream.
func PCM(samples []float32) []byte {
	return wav.PCM(samples)
}
