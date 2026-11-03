# PDGFR

`pdgfr` is a **zero-install, pure-CPU French text-to-speech binary**. One static
Go executable turns text into 22,050 Hz audio: no Python library, no ONNX
Runtime, no model server, no GPU, and no file to install.

* 22,050 Hz mono WAV, Brindille: ~94k parameters, one voice (Estelle)
* the model, the Lex grapheme-to-phoneme model, the pronunciation dictionary,
  the synthesizer constants and the Alkove runtime are embedded; the binary
  reads nothing from disk
* streaming: the text is read sentence by sentence, so playback starts after
  the first sentence, not after the paragraph
* pure Go, `CGO_ENABLED=0`, every OS/arch Go supports; the inference module is
  WebAssembly executed in-process by [wazero](https://wazero.io)
* ~50× realtime on a desktop CPU; ~20 ms to load the engine and the model

The model is **Brindille** by Eddy Bordi, a French TTS that runs on the
[Alkove](https://github.com/alkove-ai/alkove) engine.

## Install

Prebuilt archives for **Linux, macOS and Windows** (amd64 and arm64) are on the
[releases page](https://github.com/akhenak/pdgfr/releases). They are fully
static and embed everything.

Homebrew (macOS):

```sh
brew install akhenakh/tap/pdgfr
```

Homebrew on Linux is not supported; Linux users install the release tarball
directly.

Or build it (Go 1.27 or newer):

```sh
make build              # -> ./pdgfr
go build -trimpath -o pdgfr ./cmd/pdgfr
```

## Use

```sh
pdgfr "Bonjour à tous."                      # plays on the default device
pdgfr -o bonjour.wav "Bonjour à tous."       # writes a file instead
pdgfr -speed 1.25 "Un peu plus vite."
pdgfr -pitch -2 -intonation 1.1 "Plus grave."
pdgfr -o - "Bonjour." > bonjour.wav          # streaming WAV on stdout
echo "Un texte plus long..." | pdgfr         # reads stdin
pdgfr -phonemes "Bonjour à tous."            # prints the phonemes to stderr
pdgfr -play=false "Bonjour."                 # writes pdgfr.wav
```

On a headless machine with no audio device, pass `-o FILE` or `-o -`; otherwise
the default is to play.

### Notify when a long command finishes

```sh
sleep 20 && pdgfr "commande terminée"
make build && pdgfr "le build est fini"
```

### Flags

| | |
|---|---|
| `-o` | output WAV file (`-` = stdout; setting it disables playback) |
| `-play` | play audio (default true; `-play=false` writes `pdgfr.wav`) |
| `-speed` | speaking rate, 0.5 to 2 (2 = twice as fast) |
| `-pitch` | voice pitch in semitones |
| `-intonation` | intonation range, 1 = the model's |
| `-treble` | treble gain in dB |
| `-breath` | breath gain in dB |
| `-gap` | silence between sentences in ms (default 150) |
| `-chunk` | audio slice handed to the player in ms (default 200) |
| `-phonemes` | print each sentence's phonemes to stderr |
| `-quiet` | do not print statistics |
| `-version` | print version |

`speed` is clamped by the model to 0.5–2. The engine is deterministic: the
same text and options give the same samples, whether you call `Synthesize` or
`Stream`.

## Use as a library

The module root is the public API:

```go
import (
	"context"
	"os"

	"github.com/akhenak/pdgfr"
)

e, err := pdgfr.New()
if err != nil {
	panic(err)
}
defer e.Close()

audio, err := e.Synthesize(context.Background(), "Bonjour à tous.",
	pdgfr.WithSpeed(1.1), pdgfr.WithVoice(-2, 1, 0, 0))
if err != nil {
	panic(err)
}
f, _ := os.Create("bonjour.wav")
defer f.Close()
if err := pdgfr.WriteWAV(f, audio); err != nil {
	panic(err)
}
```

`Synthesize` returns mono `float32` at `pdgfr.SampleRate` (22,050 Hz). For early
playback, stream instead:

```go
err = e.Stream(ctx, text, func(chunk []float32) error {
	_, err := player.Write(pdgfr.PCM(chunk)) // raw s16le, 22,050 Hz mono
	return err
}, pdgfr.WithSpeed(1))
```

Options: `WithSpeed`, `WithVoice`, `WithGap` (silence between sentences),
`WithChunk` (slice size), `WithPhonemes` (a diagnostic callback). An `Engine`
is safe for concurrent use; cancellation is checked between slices and
sentences. `e.Phonemes` returns the sequence Brindille pronounces. See
`examples/hello` for a complete program.

## Performance

Measured on one AMD Ryzen 9 7900X, CPU only (end to end, including the text
frontend and WAV writing):

| | |
|---|---|
| throughput, 20-sentence paragraph (55 s of audio) | ~49× realtime |
| first audio, that paragraph | 56 ms (after the first sentence) |
| throughput, one 6.3 s sentence | ~34× realtime |
| first audio, one short sentence | ~28 ms |
| engine + model load | ~20 ms |
| binary | 8.8 MB, of which 2.7 MB embedded |

The model is tiny; the cost is the WebAssembly engine plus the frontend. The
engine compiles the module once at startup (part of the 20 ms), then runs it
natively.

## How it works

```
text ──► sentence split (Go) ──► tts.wasm ──► 22,050 Hz WAV
                                   │
                                   ├─ Alkove engine: the .alk runtime
                                   ├─ the French frontend: numbers, Lex, dictionary
                                   └─ the harmonic + noise synthesizer
```

* **`internal/assets`** embeds the published Brindille files, pinned to the
  Hugging Face revision `d393e3d`: `tts.wasm`
  (345 KB), `tts.alk` (266 KB), `lex.alk` (1.8 MB), `fr.dict.bin` (236 KB),
  `tts.params.bin` (916 B) and `tts.json` (the symbol table). The files are
  pinned by SHA-256 in `internal/assets/assets_test.go`; `make assets`
  re-downloads them from the same Hugging Face revision.
* **`internal/tts`** starts the WebAssembly module with wazero, hands it the
  four files through the `tts_*` C ABI, and speaks one sentence at a time.
  Before each sentence it restores a snapshot of the module's memory taken
  after loading, which resets the synthesizer's noise state and makes every
  call reproducible.
* **`internal/wav`** writes mono 16-bit PCM WAV, including streaming output.
* **`cmd/pdgfr`** handles flags, stdin, playback (pipewire, PulseAudio, ALSA,
  ffplay; `afplay` on macOS; `SoundPlayer` on Windows) and statistics.

The text frontend, Lex and the synthesizer are inside `tts.wasm`, the artifact
published with the model. This repository does not rebuild it; see
`scripts/fetch-assets.sh` for the pinned inputs.

### Streaming

Brindille synthesizes a whole utterance and returns it in one call, so the
granularity here is the sentence:

* the text is split after `. ! ? …`, keeping titles and initials ("M. Dupont",
  "J. R. R. Tolkien") together;
* a sentence longer than 500 runes is cut at a comma or a semicolon, so a
  single huge sentence cannot delay the first sound;
* each sentence's audio is handed to the player in 200 ms slices;
* a 150 ms silence separates sentences; with it, reading three sentences gives
  exactly the sample count of Brindille reading the whole paragraph in one
  call.

## Limits

These are Brindille's, not the port's:

* French only, one voice (Estelle), no voice cloning;
* integers and decimals are read (with a comma), abbreviations, acronyms and
  symbols are not expanded;
* liaisons follow rules for the most common words only;
* the sound is slightly buzzy and muffled compared with natural speech;
* names and foreign words go through Lex, which can miss.

## Licensing

The Go code in this repository is **Apache-2.0** (see `LICENSE`). The embedded
components keep their own terms, all compatible with redistribution; the full
notices are in [`NOTICE`](NOTICE) and [`LICENSES.md`](LICENSES.md):

* Brindille model, `tts.wasm`, Lex and the Alkove engine — Apache-2.0,
  Eddy Bordi / [Alkove](https://github.com/alkove-ai/alkove);
* `fr.dict.bin` — derived from the Montreal Forced Aligner French dictionary,
  CC BY 4.0;
* the Estelle voice — Pocket TTS by Kyutai, CC BY 4.0 (recording CC0);
* network and synthesizer after AuroraTTS-100K — Apache-2.0;
* Unicode character tables — Unicode License v3.

Unlike `pdg`, the shipped binary embeds no GPL component.

## Credits

* **Brindille** and the Alkove engine: [Eddy Bordi](https://huggingface.co/edbrdi).
  Model: <https://huggingface.co/edbrdi/brindille>.
* **Alkove**: <https://github.com/alkove-ai/alkove> — the `.alk` format and its
  reference engine, Apache-2.0.
* Voice **Estelle** from [Pocket TTS](https://github.com/kyutai-labs/pocket-tts)
  by Kyutai; architecture after
  [AuroraTTS-100K](https://huggingface.co/AuroraAI-Research/AuroraTTS-100K).
* [wazero](https://wazero.io) — the pure-Go WebAssembly runtime.

## Releasing

Push a tag (`vX.Y.Z`). `.github/workflows/release.yml` runs GoReleaser for
Linux, macOS and Windows on amd64/arm64, attests the archives with GitHub
build provenance, then `scripts/brew-bump.sh` updates
`akhenakh/homebrew-tap` with a macOS-only formula. Configure a
`TAP_GITHUB_TOKEN` repository secret with `contents: write` on the tap.
