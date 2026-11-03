package pdgfr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const sampleText = "Bonjour à tous. Comment vas-tu ? Je vais bien, merci."

func TestSynthesize(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	audio, err := e.Synthesize(context.Background(), sampleText)
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) == 0 {
		t.Fatal("no audio")
	}
	secs := float64(len(audio)) / SampleRate
	if secs < 1 || secs > 15 {
		t.Errorf("%.2f s of audio for %d characters", secs, len(sampleText))
	}
}

func TestStreamMatchesSynthesize(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	opts := []Option{WithSpeed(1.2), WithGap(80 * time.Millisecond), WithChunk(37 * time.Millisecond)}
	want, err := e.Synthesize(context.Background(), sampleText, opts...)
	if err != nil {
		t.Fatal(err)
	}
	var got []float32
	if err := e.Stream(context.Background(), sampleText, func(chunk []float32) error {
		got = append(got, chunk...)
		return nil
	}, opts...); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stream gave %d samples, synthesize %d", len(got), len(want))
	}
}

func TestGap(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	flat, err := e.Synthesize(context.Background(), "Bonjour. Au revoir. À bientôt.", WithGap(0))
	if err != nil {
		t.Fatal(err)
	}
	gapped, err := e.Synthesize(context.Background(), "Bonjour. Au revoir. À bientôt.", WithGap(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	want := 2 * samples(200*time.Millisecond) // three sentences, two gaps
	if diff := len(gapped) - len(flat); diff != want {
		t.Errorf("gap added %d samples, want %d", diff, want)
	}
}

func TestPhonemes(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	ph, err := e.Phonemes(context.Background(), "Bonjour à tous.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ph, "bɔ̃") {
		t.Errorf("unexpected phonemes: %q", ph)
	}
}

func TestNoSpeech(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if _, err := e.Synthesize(context.Background(), "   "); !errors.Is(err, ErrNoSpeech) {
		t.Errorf("got %v, want ErrNoSpeech", err)
	}
	if err := e.Stream(context.Background(), "   ", func([]float32) error { return nil }); !errors.Is(err, ErrNoSpeech) {
		t.Errorf("got %v, want ErrNoSpeech", err)
	}
}

func TestCancelled(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Synthesize(ctx, sampleText); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestYieldError(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	stop := errors.New("stop")
	err = e.Stream(context.Background(), sampleText, func([]float32) error { return stop })
	if !errors.Is(err, stop) {
		t.Errorf("got %v, want %v", err, stop)
	}
}

func TestVoiceChangesAudio(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	plain, err := e.Synthesize(context.Background(), "Bonjour à tous.")
	if err != nil {
		t.Fatal(err)
	}
	low, err := e.Synthesize(context.Background(), "Bonjour à tous.", WithVoice(-6, 1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != len(low) {
		t.Fatalf("pitch changed the length: %d vs %d", len(plain), len(low))
	}
	if reflect.DeepEqual(plain, low) {
		t.Error("pitch did not change the audio")
	}
}
