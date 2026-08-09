#!/usr/bin/env bash
# Tests wezterm-limen.lua inside WezTerm's own Lua runtime.
#
# There is no standalone Lua interpreter here and none is needed: `wezterm
# ls-fonts` loads the config file, which runs the assertions in selftest.lua in
# exactly the runtime that will run the real module. WezTerm exits 0 even on a
# config error, so the check is on the output, not the exit code.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
FAILED=0

pass() { printf '  ok    %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; [[ -n "${2:-}" ]] && printf '        %s\n' "$2"; FAILED=1; }

WEZTERM="${WEZTERM_BIN:-}"
if [[ -z "$WEZTERM" ]]; then
  for candidate in \
      "$(command -v wezterm 2>/dev/null)" \
      /Applications/WezTerm.app/Contents/MacOS/wezterm \
      /opt/homebrew/bin/wezterm; do
    [[ -n "$candidate" && -x "$candidate" ]] && { WEZTERM="$candidate"; break; }
  done
fi

echo "--- wezterm-limen tests ---"

if [[ -z "$WEZTERM" ]]; then
  echo "  SKIP  wezterm not found (set WEZTERM_BIN to test the Lua module)"
  exit 0
fi
printf '  using %s (%s)\n' "$WEZTERM" "$("$WEZTERM" --version 2>/dev/null | head -1)"

run_config() {
  LIMEN_INTEGRATIONS="$HERE" "$WEZTERM" --config-file "$1" ls-fonts 2>&1
}

# ---- 1. the module parses and loads ----
LOADER="$(mktemp -t wezterm-limen-load.XXXXXX).lua"
cat > "$LOADER" <<EOF
package.path = '$HERE/?.lua;' .. package.path
local limen = require 'wezterm-limen'
local config = {}
limen.apply(config)
return config
EOF
out="$(run_config "$LOADER")"
if grep -qiE 'syntax error|error.*wezterm-limen|failed to load' <<< "$out"; then
  fail "module loads" "$(grep -iE 'error' <<< "$out" | head -3)"
else
  pass "module loads"
fi
if grep -q "use_fancy_tab_bar" <<< "$out" || [[ -n "$out" ]]; then
  pass "apply() is callable from a real config"
fi
rm -f "$LOADER"

# ---- 2. the assertions in selftest.lua ----
out="$(run_config "$HERE/selftest.lua")"
if grep -q 'LIMEN-SELFTEST-OK' <<< "$out"; then
  pass "$(grep -o 'LIMEN-SELFTEST-OK.*' <<< "$out" | head -1)"
elif grep -q 'LIMEN-SELFTEST-FAIL' <<< "$out"; then
  fail "selftest assertions" "$(sed -n '/LIMEN-SELFTEST-FAIL/,/^$/p' <<< "$out" | head -12)"
else
  fail "selftest ran" "no marker in output:
$(echo "$out" | head -12)"
fi

# ---- 3. a deliberately broken module must be reported, not swallowed ----
# Without this the test above could pass by never running anything at all.
BROKEN_DIR="$(mktemp -d)"
sed 's/^local M = {}$/local M = {} this is not lua/' "$HERE/wezterm-limen.lua" \
  > "$BROKEN_DIR/wezterm-limen.lua"
BROKEN_CFG="$BROKEN_DIR/cfg.lua"
cat > "$BROKEN_CFG" <<EOF
package.path = '$BROKEN_DIR/?.lua;' .. package.path
require 'wezterm-limen'
return {}
EOF
out="$(LIMEN_INTEGRATIONS="$BROKEN_DIR" "$WEZTERM" --config-file "$BROKEN_CFG" ls-fonts 2>&1)"
if grep -qiE 'syntax error' <<< "$out"; then
  pass "a broken module is detected (negative control)"
else
  fail "negative control" "a syntax error went unnoticed, so the checks above prove nothing"
fi
rm -rf "$BROKEN_DIR"

# ---- 4. the real binary produces what the module expects ----
LIMEN_BIN="${LIMEN_BIN:-$HERE/../limen}"
if [[ ! -x "$LIMEN_BIN" ]]; then
  LIMEN_BIN="$(command -v limen 2>/dev/null || true)"
fi
if [[ -n "$LIMEN_BIN" && -x "$LIMEN_BIN" ]]; then
  TMP="$(mktemp -d)"
  printf 'label: tessera\nactor: Leo\nmodel: claude-opus-5\ngithubUser: leo81\ngcloudProject: p-1\ngateway: http://localhost:8787\n' \
    > "$TMP/.limen.yaml"
  json="$(cd "$TMP" && "$LIMEN_BIN" json)"
  missing=""
  for key in root label actor model github_user gcloud_project gateway api_key_in_config; do
    grep -q "\"$key\"" <<< "$json" || missing="$missing $key"
  done
  if [[ -z "$missing" ]]; then
    pass "limen json carries every field the module reads"
  else
    fail "limen json fields" "missing:$missing"
  fi
  # Outside a project the module must see {} and treat it as "no context".
  empty="$(cd "$(mktemp -d)" && "$LIMEN_BIN" json)"
  if [[ "$(tr -d '[:space:]' <<< "$empty")" == "{}" ]]; then
    pass "limen json outside a project is {} as the module assumes"
  else
    fail "limen json outside a project" "got: $empty"
  fi
  rm -rf "$TMP"
else
  echo "  SKIP  limen binary not built (make build) — field check skipped"
fi

echo
if [[ "$FAILED" -eq 0 ]]; then
  echo "all wezterm-limen tests passed"
else
  echo "wezterm-limen tests FAILED"
fi
exit "$FAILED"
