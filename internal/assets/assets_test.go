package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// The published files are pinned: if one is replaced, its digest must be
// updated here on purpose.
func TestAssets(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		sum  string
	}{
		{"tts.wasm", Wasm, "b5d0644528bcb1388ff7ec527d4fcd36eb14c6c536438e7e06d622112a176c94"},
		{"tts.alk", Model, "65e8b4063bf99c27d509c8427b61e7867e721d62e5cad4171faf795dc02f44c6"},
		{"lex.alk", Lex, "1e64ca9f5c2281cadae0f9c4a944cd1b1b433f0c0c29760c36340c97f7308957"},
		{"fr.dict.bin", Dict, "7e87af1778069123739bcd07158b2b0241854c69b33334c98cd051841a039b53"},
		{"tts.params.bin", Params, "6ddcbc28093ac11ac2bf462da20014a9feb1583fd23c8bb62a8a16ae0fe07c07"},
		{"tts.json", Meta, "bd048d8ac8b7299115b66fddef0730e9f5d52a9ccb74a69e0150fc90df7baf2a"},
	}
	for _, c := range cases {
		if len(c.data) == 0 {
			t.Errorf("%s is empty", c.name)
			continue
		}
		sum := sha256.Sum256(c.data)
		if got := hex.EncodeToString(sum[:]); got != c.sum {
			t.Errorf("%s: sha256 %s, want %s", c.name, got, c.sum)
		}
	}
	if string(Wasm[:4]) != "\x00asm" {
		t.Error("tts.wasm is not a WebAssembly module")
	}
	if string(Model[:4]) != "ALK\x00" {
		t.Error("tts.alk does not have the ALK magic")
	}
	if string(Lex[:4]) != "ALK\x00" {
		t.Error("lex.alk does not have the ALK magic")
	}
}
