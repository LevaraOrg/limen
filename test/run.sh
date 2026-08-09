#!/usr/bin/env bash
# Tests limen against throwaway directory trees. No network, no keychain writes.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LIMEN="$ROOT/bin/limen"
FAILED=0

pass() { printf '  ok    %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; printf '        %s\n' "$2"; FAILED=1; }

contains() {
  local name="$1" needle="$2" haystack="$3"
  if [[ "$haystack" == *"$needle"* ]]; then pass "$name"; else
    fail "$name" "expected to contain '$needle', got: ${haystack:-<empty>}"
  fi
}

lacks() {
  local name="$1" needle="$2" haystack="$3"
  if [[ "$haystack" != *"$needle"* ]]; then pass "$name"; else
    fail "$name" "expected NOT to contain '$needle', got: $haystack"
  fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "--- limen tests ---"

# ---- a .limen.yaml project, read from a nested directory ----
mkdir -p "$TMP/proj/deep/deeper"
cat > "$TMP/proj/.limen.yaml" <<'EOF'
# a comment line that must be ignored
label: tessera
actor: Matthias Wegner
githubUser: leo81
claudeConfigDir: ~/.claude-work
gcloudAccount: leo@example.com
gcloudProject: my-project-123
provider: anthropic
model: claude-opus-5
gateway: http://localhost:8787
keychainService: limen-anthropic
EOF

out="$(cd "$TMP/proj/deep/deeper" && "$LIMEN" show)"
contains "finds root from nested dir" "$TMP/proj" "$out"
contains "reads label"                "tessera"   "$out"
contains "reads github user"          "leo81"     "$out"
contains "expands tilde"              "$HOME/.claude-work" "$out"
contains "marks source as .limen.yaml" ".limen.yaml" "$out"

out="$(cd "$TMP/proj" && "$LIMEN" shell)"
contains "exports CLAUDE_CONFIG_DIR"       "export CLAUDE_CONFIG_DIR='$HOME/.claude-work'" "$out"
contains "exports CLOUDSDK_CORE_ACCOUNT"   "CLOUDSDK_CORE_ACCOUNT='leo@example.com'" "$out"
contains "exports CLOUDSDK_CORE_PROJECT"   "CLOUDSDK_CORE_PROJECT='my-project-123'" "$out"
contains "exports ANTHROPIC_BASE_URL"      "ANTHROPIC_BASE_URL='http://localhost:8787'" "$out"
contains "exports LIMEN_ROOT"              "LIMEN_ROOT='$TMP/proj'" "$out"
# The hook must get by with one call, so the prompt segment rides along here.
contains "exports LIMEN_SEGMENT"           "LIMEN_SEGMENT='tessera · leo81 · claude-opus-5'" "$out"

out="$(cd "$TMP/proj" && "$LIMEN" json)"
contains "json has root"    "\"root\":\"$TMP/proj\"" "$out"
contains "json has gateway" '"gateway":"http://localhost:8787"' "$out"
contains "json flags no plaintext key" '"api_key_in_config":false' "$out"

out="$(cd "$TMP/proj" && "$LIMEN" prompt)"
contains "prompt segment" "tessera · leo81 · claude-opus-5" "$out"

# ---- empty fields must not be exported ----
mkdir -p "$TMP/sparse"
printf 'label: sparse\nprovider: anthropic\n' > "$TMP/sparse/.limen.yaml"
out="$(cd "$TMP/sparse" && "$LIMEN" shell)"
lacks "omits empty CLAUDE_CONFIG_DIR" "CLAUDE_CONFIG_DIR" "$out"
lacks "omits empty CLOUDSDK"          "CLOUDSDK"          "$out"
contains "still exports label"        "LIMEN_LABEL='sparse'" "$out"

# ---- legacy .orca/ trees keep loading ----
mkdir -p "$TMP/legacy/.orca" "$TMP/legacy/src/main"
printf -- '---\nprovider: anthropic\nmodel: claude-opus-4-5\n' > "$TMP/legacy/.orca/config.yaml"
printf -- '---\nactorId: "abc-123"\nname: "Leo"\n' > "$TMP/legacy/.orca/identity.yaml"
out="$(cd "$TMP/legacy/src/main" && "$LIMEN" show)"
contains "legacy: finds .orca root" "$TMP/legacy" "$out"
contains "legacy: reads name"       "Leo"         "$out"
contains "legacy: reads model"      "claude-opus-4-5" "$out"
contains "legacy: labels source"    "legacy"      "$out"

# ---- plaintext key is flagged, never printed ----
mkdir -p "$TMP/leaky"
printf 'label: leaky\nprovider: anthropic\napiKey: sk-ant-SECRETVALUE\n' > "$TMP/leaky/.limen.yaml"
out="$(cd "$TMP/leaky" && "$LIMEN" show)"
contains "warns about plaintext key" "Klartextschlüssel" "$out"
lacks    "show never prints the key" "SECRETVALUE"       "$out"
out="$(cd "$TMP/leaky" && "$LIMEN" json)"
contains "json flags plaintext key" '"api_key_in_config":true' "$out"
lacks    "json never prints the key" "SECRETVALUE" "$out"
out="$(cd "$TMP/leaky" && "$LIMEN" prompt)"
contains "prompt marks the leak" "!key-in-config" "$out"

# ---- quoting and comments ----
mkdir -p "$TMP/quoted"
cat > "$TMP/quoted/.limen.yaml" <<'EOF'
label: "quoted label"
actor: 'single quoted'
model: claude-opus-5   # trailing comment must go
EOF
out="$(cd "$TMP/quoted" && "$LIMEN" show)"
contains "strips double quotes"   "quoted label"   "$out"
contains "strips single quotes"   "single quoted"  "$out"
contains "strips trailing comment" "model:        claude-opus-5" "$out"
lacks    "comment text is gone"    "must go"       "$out"

# ---- no context at all: quiet, exit 0, so shell startup files can call it ----
mkdir -p "$TMP/nothing"
out="$(cd "$TMP/nothing" && "$LIMEN" json)"
contains "json is {} without context" "{}" "$out"
if (cd "$TMP/nothing" && "$LIMEN" shell >/dev/null 2>&1); then
  pass "shell exits 0 without context"
else
  fail "shell exits 0 without context" "non-zero exit would break .zshrc"
fi
out="$(cd "$TMP/nothing" && "$LIMEN" shell 2>/dev/null)"
if [[ -z "$out" ]]; then pass "shell is silent without context"; else
  fail "shell is silent without context" "got: $out"
fi
if (cd "$TMP/nothing" && "$LIMEN" show >/dev/null 2>&1); then
  fail "show errors without context" "expected non-zero exit"
else
  pass "show errors without context"
fi

# ---- init and hook ----
mkdir -p "$TMP/fresh"
(cd "$TMP/fresh" && "$LIMEN" init >/dev/null)
if [[ -f "$TMP/fresh/.limen.yaml" ]]; then pass "init writes .limen.yaml"; else
  fail "init writes .limen.yaml" "file missing"
fi
if (cd "$TMP/fresh" && "$LIMEN" init >/dev/null 2>&1); then
  fail "init refuses to overwrite" "expected non-zero exit"
else
  pass "init refuses to overwrite"
fi
contains "zsh hook mentions chpwd" "add-zsh-hook chpwd" "$("$LIMEN" hook zsh)"
contains "bash hook mentions PROMPT_COMMAND" "PROMPT_COMMAND" "$("$LIMEN" hook bash)"

echo
if [[ "$FAILED" -eq 0 ]]; then echo "all limen tests passed"; else echo "limen tests FAILED"; fi
exit "$FAILED"
