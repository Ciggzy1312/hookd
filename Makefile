.PHONY: build test deps-proof clean

GO ?= go
BIN := hookd

build:
	$(GO) build -o $(BIN) ./cmd/hookd

test:
	$(GO) test ./...

deps-proof:
	$(GO) list -m all > deps-proof.txt
	$(GO) version >> deps-proof.txt

clean:
	rm -f $(BIN) hookd-a hookd-b
