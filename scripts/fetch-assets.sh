#!/bin/sh
# Download the pinned Brindille files and verify their digests.
#
# The WebAssembly module, the two networks, the dictionary and the synthesizer
# constants are all that pdgfr runs. They come from one Hugging Face revision:
# update REV and the digests together, on purpose.
set -eu

REV=d393e3d5161937f2064bcfef33213749af4979f0
BASE="https://huggingface.co/edbrdi/brindille/resolve/$REV"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIR=$ROOT/internal/assets

if command -v sha256sum >/dev/null 2>&1; then
	SUM="sha256sum"
else
	SUM="shasum -a 256"
fi

fetch() {
	path=$1
	out=$DIR/$(basename "$path")
	printf '%s: ' "$out"
	curl -fsSL "$BASE/$path" -o "$out"
	echo ok
}

mkdir -p "$DIR"
fetch tts.wasm
fetch model/tts.alk
fetch model/lex.alk
fetch model/fr.dict.bin
fetch model/tts.params.bin
fetch model/tts.json
printf '%s: ' "$ROOT/NOTICE"
curl -fsSL "$BASE/NOTICE" -o "$ROOT/NOTICE"
echo ok

cd "$DIR"
$SUM -c <<'EOF'
b5d0644528bcb1388ff7ec527d4fcd36eb14c6c536438e7e06d622112a176c94  tts.wasm
65e8b4063bf99c27d509c8427b61e7867e721d62e5cad4171faf795dc02f44c6  tts.alk
1e64ca9f5c2281cadae0f9c4a944cd1b1b433f0c0c29760c36340c97f7308957  lex.alk
7e87af1778069123739bcd07158b2b0241854c69b33334c98cd051841a039b53  fr.dict.bin
6ddcbc28093ac11ac2bf462da20014a9feb1583fd23c8bb62a8a16ae0fe07c07  tts.params.bin
bd048d8ac8b7299115b66fddef0730e9f5d52a9ccb74a69e0150fc90df7baf2a  tts.json
EOF
cd "$ROOT"
$SUM -c <<'EOF'
4e211d8cafc54570eb2ff639e4489bcc519ce1aa3c836cf1c07ee0d84f12450d  NOTICE
EOF
