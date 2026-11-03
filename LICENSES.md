# Licenses

## Project code

The Go code in this repository is **Apache-2.0** (see `LICENSE`). It embeds the
published Brindille files described below; each keeps its own license. All of
them allow redistribution, and unlike `pdg` the shipped binary contains no GPL
component.

## Embedded model and runtime

* **Brindille** (`tts.alk`, `tts.wasm`, `tts.params.bin`) — Apache-2.0,
  Copyright 2026 Eddy Bordi. <https://huggingface.co/edbrdi/brindille>.
* **Lex** (`lex.alk`) — Apache-2.0, Copyright 2026 Eddy Bordi.
  <https://huggingface.co/alkove-ai/alkove-lex-1-preview>.
* **Alkove engine and core** — Apache-2.0, Copyright 2026 Eddy Bordi.
  <https://github.com/alkove-ai/alkove>. The Unicode character tables of
  alkove-core derive from the Unicode Character Database 16.0, under the
  Unicode License v3 (reproduced in `NOTICE`).
* **`fr.dict.bin`** — derived from the French dictionary of the Montreal Forced
  Aligner (<https://github.com/MontrealCorpusTools/mfa-models>), CC BY 4.0.
  Changes: pronunciations mapped to Brindille's phoneme set, reduced to 19,662
  words.
* **The Estelle voice** — Pocket TTS (<https://github.com/kyutai-labs/pocket-tts>)
  by Kyutai, CC BY 4.0; the Estelle recording
  (<https://huggingface.co/kyutai/tts-voices>) is Kyutai's, released under CC0.
* **Network and synthesizer architecture** — after AuroraTTS-100K
  (<https://huggingface.co/AuroraAI-Research/AuroraTTS-100K>) by
  AuroraAI-Research, Apache-2.0.

The full notices are in [`NOTICE`](NOTICE), as published with the model.

## Other dependencies

* [wazero](https://github.com/tetratelabs/wazero) — Apache-2.0, the only Go
  dependency. It executes the embedded WebAssembly module in-process.
* `golang.org/x/sys` — BSD-3-Clause (indirect, through wazero).
