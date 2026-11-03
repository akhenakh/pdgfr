// Command hello shows how to use pdgfr as a library.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/akhenak/pdgfr"
)

func main() {
	e, err := pdgfr.New()
	if err != nil {
		panic(err)
	}
	defer e.Close()

	audio, err := e.Synthesize(context.Background(), "Bonjour depuis la bibliothèque pdgfr.",
		pdgfr.WithSpeed(1), pdgfr.WithVoice(0, 1, 0, 0))
	if err != nil {
		panic(err)
	}
	f, err := os.Create("bonjour.wav")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := pdgfr.WriteWAV(f, audio); err != nil {
		panic(err)
	}
	fmt.Printf("bonjour.wav: %.1f s\n", float64(len(audio))/pdgfr.SampleRate)
}
