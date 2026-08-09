BINARY  := limen
PREFIX  ?= $(HOME)/.local/bin
GOFLAGS := -trimpath -ldflags "-s -w"

.PHONY: all build test vet fmt cover bench install uninstall clean

all: vet test build

build:
	go build $(GOFLAGS) -o $(BINARY) .
	@echo "built ./$(BINARY) ($$(du -h $(BINARY) | cut -f1))"

test:
	go test ./...

# Verbose, so a human can read which behaviour is covered.
test-v:
	go test -v ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	@echo "detail: go tool cover -html=coverage.out"

# Times the paths the shell hook actually uses. The keychain lookup is a
# security(1) fork and dominates everything else, so it is measured separately.
bench: build
	@scripts/bench.sh ./$(BINARY)

install: build
	@mkdir -p $(PREFIX)
	install -m 0755 $(BINARY) $(PREFIX)/$(BINARY)
	@echo "installed $(PREFIX)/$(BINARY)"
	@case ":$$PATH:" in *":$(PREFIX):"*) ;; *) echo "note: $(PREFIX) is not on your PATH" ;; esac
	@echo
	@echo "Add the hook to your shell:"
	@echo "    eval \"\$$($(BINARY) hook zsh)\""

uninstall:
	rm -f $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY) coverage.out
