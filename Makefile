.PHONY: build test deps-proof reproduce clean

GO ?= go
BIN := hookd
REPRO_FLAGS := -trimpath -ldflags="-buildid="

build:
	$(GO) build -o $(BIN) ./cmd/hookd

test:
	$(GO) test ./...

deps-proof:
	$(GO) list -m all > deps-proof.txt
	$(GO) version >> deps-proof.txt

reproduce:
	CGO_ENABLED=0 $(GO) build $(REPRO_FLAGS) -o hookd-a ./cmd/hookd
	CGO_ENABLED=0 $(GO) build $(REPRO_FLAGS) -o hookd-b ./cmd/hookd
	@if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum hookd-a hookd-b; \
	else \
		shasum -a 256 hookd-a hookd-b; \
	fi
	cmp hookd-a hookd-b

clean:
	rm -f $(BIN) hookd-a hookd-b

