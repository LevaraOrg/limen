#!/usr/bin/env bash
# Times the subcommands the shell hook uses, against a throwaway project.
#
# The point of the split: everything except a keychain lookup is process-spawn
# cost plus about a millisecond. The keychain lookup is a security(1) fork and
# costs ~20 ms, which is macOS, not Limen. /usr/bin/true is measured too so the
# floor of your machine is visible rather than implied.
set -euo pipefail

BIN="${1:-./limen}"
N="${N:-200}"
[[ -x "$BIN" ]] || { echo "usage: $0 /path/to/limen" >&2; exit 1; }
BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cd "$TMP"

time_it() {
  local label="$1"; shift
  local start end
  start=$(python3 -c 'import time;print(time.time())')
  for _ in $(seq "$N"); do "$@" >/dev/null 2>&1 || true; done
  end=$(python3 -c 'import time;print(time.time())')
  python3 -c "print('%-38s %7.2f ms' % ('$label', ($end-$start)*1000/$N))"
}

printf 'limen bench — %s runs per row\n\n' "$N"

printf 'label: bench\nactor: Leo\ngithubUser: leo81\nmodel: claude-opus-5\nprovider: anthropic\n' > .limen.yaml
time_it "root"                          "$BIN" root
time_it "prompt   (hook, status line)"  "$BIN" prompt
time_it "shell    (keychain miss)"      "$BIN" shell
ANTHROPIC_API_KEY=sk-bench time_it "shell    (key already in env)" "$BIN" shell

printf 'label: bench\nprovider: ollama\nmodel: qwen2.5\n' > .limen.yaml
time_it "shell    (provider != anthropic)" "$BIN" shell

echo
time_it "/usr/bin/true (spawn floor)"   /usr/bin/true
echo
echo "Anything within ~1.5 ms of the floor is process spawn, not Limen."
echo "The keychain row is a security(1) fork; export the key once to avoid it."
