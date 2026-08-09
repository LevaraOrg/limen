BINARY  := limen
PREFIX  ?= $(HOME)/.local/bin
GOFLAGS := -trimpath -ldflags "-s -w"

.PHONY: all build test test-go test-wezterm test-v vet fmt cover bench install install-wezterm uninstall clean

all: vet test build

build:
	go build $(GOFLAGS) -o $(BINARY) .
	@echo "built ./$(BINARY) ($$(du -h $(BINARY) | cut -f1))"

# Go tests plus the WezTerm Lua module, which is tested inside WezTerm's own
# Lua runtime because there is no standalone interpreter to test it with.
test: test-go test-wezterm

test-go:
	go test ./...

test-wezterm: build
	@./integrations/test.sh

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

# Symlinks the WezTerm module next to your wezterm.lua. WezTerm spawns child
# processes without your shell environment, so the absolute path to the binary is
# baked into the suggested snippet.
WEZTERM_DIR ?= $(HOME)/.config/wezterm

install-wezterm:
	@mkdir -p $(WEZTERM_DIR)
	ln -sf $(CURDIR)/integrations/wezterm-limen.lua $(WEZTERM_DIR)/wezterm-limen.lua
	@echo "linked $(WEZTERM_DIR)/wezterm-limen.lua"
	@echo
	@echo "Add to $(WEZTERM_DIR)/wezterm.lua:"
	@echo
	@echo "    local limen = require 'wezterm-limen'"
	@echo "    limen.apply(config)"
	@echo
	@echo "WezTerm does not inherit your shell PATH. If limen is not on its PATH:"
	@echo
	@echo "    local limen = require 'wezterm-limen'"
	@echo "    limen.bin = '$(PREFIX)/$(BINARY)'"
	@echo "    limen.apply(config)"

uninstall:
	rm -f $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY) coverage.out
