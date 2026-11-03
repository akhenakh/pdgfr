GO ?= go
BIN ?= pdgfr

.PHONY: build test fmt assets clean

# The shipped build: one static binary, all assets embedded.
build:
	$(GO) build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/pdgfr

test:
	$(GO) test ./...

# Re-download the pinned Brindille files from Hugging Face.
assets:
	./scripts/fetch-assets.sh

fmt:
	gofmt -w ./cmd ./internal ./examples ./*.go

clean:
	rm -f $(BIN) pdgfr.wav
