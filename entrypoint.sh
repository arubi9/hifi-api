#!/bin/sh
set -e

MODE="${1:-tui}"

case "$MODE" in
  api)
    # API-only mode: just run uvicorn
    exec uvicorn main:app --host 0.0.0.0 --port 8000
    ;;
  tui)
    # TUI connects to the public API directly (no local server needed)
    exec hifi-tui
    ;;
  *)
    echo "Usage: docker run -it hifi-api [tui|api]"
    echo "  tui  - Launch interactive TUI (default)"
    echo "  api  - Run API server only (headless)"
    exit 1
    ;;
esac
