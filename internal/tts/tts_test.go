package tts

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Bonjour à tous.", []string{"Bonjour à tous."}},
		{"Bonjour. Comment vas-tu ?", []string{"Bonjour.", "Comment vas-tu ?"}},
		{"M. Dupont est là. Il attend.", []string{"M. Dupont est là.", "Il attend."}},
		{"J. R. R. Tolkien écrit.", []string{"J. R. R. Tolkien écrit."}},
		{"« Bonjour ! » dit-elle. Ensuite, elle part.", []string{"« Bonjour ! » dit-elle.", "Ensuite, elle part."}},
		{"Une phrase sans fin", []string{"Une phrase sans fin"}},
		{"   ", nil},
	}
	for _, c := range cases {
		if got := SplitSentences(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitSentences(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestSplitLong(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString("Ceci est un membre de phrase, ")
	}
	b.WriteString("et la fin.")
	parts := SplitSentences(b.String())
	if len(parts) < 2 {
		t.Fatalf("long sentence was not split: %d parts", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > maxSentenceRunes {
			t.Errorf("part of %d runes exceeds %d", n, maxSentenceRunes)
		}
	}
}

func TestSpeak(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	res, err := e.Speak("Bonjour à tous.", 1, DefaultVoice)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Audio) == 0 {
		t.Fatal("no audio")
	}
	for i, s := range res.Audio {
		if s < -4 || s > 4 {
			t.Fatalf("sample %d out of range: %v", i, s)
		}
	}
	if !strings.Contains(res.Phonemes, "bɔ̃") {
		t.Errorf("phonemes %q do not look French", res.Phonemes)
	}

	again, err := e.Speak("Bonjour à tous.", 1, DefaultVoice)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Audio, again.Audio) {
		t.Error("two identical calls gave different audio")
	}
}

func TestSpeakSpeedClamp(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	slow, err := e.Speak("Bonjour à tous.", 0.1, DefaultVoice)
	if err != nil {
		t.Fatal(err)
	}
	fast, err := e.Speak("Bonjour à tous.", 9, DefaultVoice)
	if err != nil {
		t.Fatal(err)
	}
	if len(slow.Audio) <= len(fast.Audio) {
		t.Errorf("speed was not clamped: %d samples at 0.1, %d at 9", len(slow.Audio), len(fast.Audio))
	}
}

func TestSpeakEmpty(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.Speak("   ", 1, DefaultVoice); err != ErrNoSpeech {
		t.Errorf("got %v, want ErrNoSpeech", err)
	}
}

func TestConcurrent(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var wg sync.WaitGroup
	for _, text := range []string{"Bonjour.", "Comment vas-tu ?", "Très bien, merci."} {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			res, err := e.Speak(text, 1, DefaultVoice)
			if err != nil {
				t.Errorf("%q: %v", text, err)
				return
			}
			if len(res.Audio) == 0 {
				t.Errorf("%q: no audio", text)
			}
		}(text)
	}
	wg.Wait()
}
