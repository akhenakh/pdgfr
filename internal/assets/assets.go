// Package assets embeds the Brindille model and the Alkove runtime.
//
// Everything the engine reads at run time lives here: the WebAssembly module
// (the Alkove engine, the French text frontend and the harmonic + noise
// synthesizer), the two .alk networks, the pronunciation dictionary and the
// synthesizer constants. Nothing is read from disk.
package assets

import _ "embed"

var (
	// Wasm is the Alkove runtime compiled to WebAssembly. It exports the
	// tts_* C ABI and needs no imports.
	//go:embed tts.wasm
	Wasm []byte

	// Model is the Brindille network (tts.alk): two plans, encoder and
	// decoder.
	//go:embed tts.alk
	Model []byte

	// Lex is the Lex grapheme-to-phoneme model (lex.alk).
	//go:embed lex.alk
	Lex []byte

	// Dict holds the 19,662 word pronunciations Lex is not asked for.
	//go:embed fr.dict.bin
	Dict []byte

	// Params holds the synthesizer and frontend constants.
	//go:embed tts.params.bin
	Params []byte

	// Meta is tts.json: the sample rate and the symbol table.
	//go:embed tts.json
	Meta []byte
)
