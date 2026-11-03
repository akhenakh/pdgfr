// Command pdgfr speaks French with Brindille, entirely on the CPU.
//
//	pdgfr "Bonjour à tous."             -> plays on the default device
//	pdgfr -o bonjour.wav "Bonjour."     -> bonjour.wav
//	echo "Texte long" | pdgfr -o - > a.wav
//	pdgfr -play=false "Bonjour."        -> pdgfr.wav
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/akhenak/pdgfr"
	"github.com/akhenak/pdgfr/internal/wav"
)

// version is set by GoReleaser (-X main.version=...); local builds say dev.
var version = "dev"

func main() {
	out := flag.String("o", "", "output WAV file instead of playing (- for stdout)")
	speed := flag.Float64("speed", 1, "speaking rate, 0.5 to 2 (2 = twice as fast)")
	pitch := flag.Float64("pitch", 0, "voice pitch in semitones")
	intonation := flag.Float64("intonation", 1, "intonation range around 1")
	treble := flag.Float64("treble", 0, "treble gain in dB")
	breath := flag.Float64("breath", 0, "breath gain in dB")
	gap := flag.Int("gap", 150, "silence between sentences in ms")
	chunk := flag.Int("chunk", 200, "audio slice handed to the player in ms")
	play := flag.Bool("play", true, "play audio (default; ignored when -o is set)")
	phonemes := flag.Bool("phonemes", false, "print the phonemes to stderr")
	quiet := flag.Bool("quiet", false, "do not print statistics")
	ver := flag.Bool("version", false, "print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pdgfr %s - Brindille French text to speech\n\nusage: pdgfr [flags] [text...]\n\nflags:\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *ver {
		fmt.Println("pdgfr", version)
		return
	}

	text := strings.Join(flag.Args(), " ")
	if text == "" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		text = string(b)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		flag.Usage()
		os.Exit(2)
	}

	e, err := pdgfr.New()
	if err != nil {
		fatal(err)
	}
	defer e.Close()

	opts := []pdgfr.Option{
		pdgfr.WithSpeed(float32(*speed)),
		pdgfr.WithVoice(float32(*pitch), float32(*intonation), float32(*treble), float32(*breath)),
		pdgfr.WithGap(time.Duration(*gap) * time.Millisecond),
		pdgfr.WithChunk(time.Duration(*chunk) * time.Millisecond),
	}
	if *phonemes {
		opts = append(opts, pdgfr.WithPhonemes(func(_, ph string) {
			fmt.Fprintln(os.Stderr, ph)
		}))
	}

	var sink func([]float32) error
	var finish func() error
	var start time.Time
	var playAt time.Duration
	bufferedPlay := false
	path := *out
	if !*play && path == "" {
		path = "pdgfr.wav"
	}
	if *play && path == "" {
		if p, err := startStreamingPlayer(pdgfr.SampleRate); err == nil {
			// Keep the encoded audio while streaming: if the player dies
			// (unsupported flags, missing audio device, ...), restart
			// playback from the beginning through the buffered player
			// instead of failing with a broken pipe.
			fallback := canPlayBuffered()
			var streamed []byte
			sink = func(samples []float32) error {
				pcm := pdgfr.PCM(samples)
				if fallback {
					streamed = append(streamed, pcm...)
				}
				if p == nil {
					return nil
				}
				if _, err := p.stdin.Write(pcm); err != nil {
					p.abandon()
					p = nil
					if !fallback {
						return err
					}
					bufferedPlay = true
				}
				return nil
			}
			finish = func() error {
				if p == nil {
					playAt = time.Since(start)
					return playBufferedPCM(streamed, pdgfr.SampleRate)
				}
				return p.close()
			}
		} else if canPlayBuffered() {
			// macOS afplay and Windows PowerShell's SoundPlayer need a
			// complete file, so buffer and play once synthesis is done.
			bufferedPlay = true
			var audio []float32
			sink = func(samples []float32) error {
				audio = append(audio, samples...)
				return nil
			}
			finish = func() error {
				// afplay needs the complete file, so playback starts now.
				playAt = time.Since(start)
				return playBuffered(audio, pdgfr.SampleRate)
			}
		} else {
			fatal(fmt.Errorf("no audio player found (tried pw-play, pw-cat, paplay, aplay, ffplay, afplay, PowerShell); write a file with -o instead"))
		}
	} else if path == "-" {
		if _, err := os.Stdout.Write(wav.Header(pdgfr.SampleRate, 0xFFFFFFFF)); err != nil {
			fatal(err)
		}
		sink = func(samples []float32) error {
			_, err := os.Stdout.Write(pdgfr.PCM(samples))
			return err
		}
		finish = func() error { return nil }
	} else {
		f, err := os.Create(path)
		if err != nil {
			fatal(err)
		}
		if _, err := f.Write(wav.Header(pdgfr.SampleRate, 0)); err != nil {
			f.Close()
			fatal(err)
		}
		var bytes int
		sink = func(samples []float32) error {
			b := pdgfr.PCM(samples)
			bytes += len(b)
			_, err := f.Write(b)
			return err
		}
		finish = func() error {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				f.Close()
				return err
			}
			if _, err := f.Write(wav.Header(pdgfr.SampleRate, bytes)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			if !*quiet {
				fmt.Fprintf(os.Stderr, "pdgfr: wrote %s\n", path)
			}
			return nil
		}
	}

	start = time.Now()
	var firstAt time.Duration
	var samples int
	err = e.Stream(context.Background(), text, func(chunk []float32) error {
		if firstAt == 0 {
			firstAt = time.Since(start)
		}
		samples += len(chunk)
		return sink(chunk)
	}, opts...)
	if err != nil {
		fatal(err)
	}
	if err := finish(); err != nil {
		fatal(err)
	}
	if !*quiet {
		seconds := float64(samples) / pdgfr.SampleRate
		total := time.Since(start)
		if bufferedPlay {
			if playAt == 0 {
				playAt = total
			}
			fmt.Fprintf(os.Stderr, "pdgfr: %.1fs of audio, playback started after %v, total %v (%.0fx realtime)\n",
				seconds, playAt.Round(time.Millisecond), total.Round(time.Millisecond),
				seconds/total.Seconds())
		} else {
			fmt.Fprintf(os.Stderr, "pdgfr: %.1fs of audio, first audio after %v, total %v (%.0fx realtime)\n",
				seconds, firstAt.Round(time.Millisecond), total.Round(time.Millisecond),
				seconds/total.Seconds())
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "pdgfr:", err)
	os.Exit(1)
}
