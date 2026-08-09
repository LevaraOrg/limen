#!/usr/bin/env bash
# Symlinks limen into ~/.local/bin and prints the shell hook to add.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
TARGET_DIR="${LIMEN_PREFIX:-$HOME/.local/bin}"

mkdir -p "$TARGET_DIR"
ln -sf "$ROOT/bin/limen" "$TARGET_DIR/limen"
printf 'verlinkt: %s -> %s\n\n' "$TARGET_DIR/limen" "$ROOT/bin/limen"

case ":$PATH:" in
    *":$TARGET_DIR:"*) ;;
    *) printf 'Hinweis: %s liegt nicht im PATH.\n\n' "$TARGET_DIR" ;;
esac

SHELL_NAME="$(basename "${SHELL:-zsh}")"
case "$SHELL_NAME" in
    bash) RC="$HOME/.bashrc"; HOOK='eval "$(limen hook bash)"' ;;
    *)    RC="$HOME/.zshrc";  HOOK='eval "$(limen hook zsh)"' ;;
esac

if [[ -f "$RC" ]] && grep -q 'limen hook' "$RC"; then
    printf 'Hook ist in %s bereits eingebunden.\n' "$RC"
else
    printf 'Noch in %s aufnehmen:\n\n    %s\n\n' "$RC" "$HOOK"
    printf 'Optional die Statuszeile:\n\n    RPROMPT='"'"'%%F{244}${LIMEN_SEGMENT}%%f'"'"'\n'
fi
