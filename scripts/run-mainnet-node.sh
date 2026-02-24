#!/bin/bash
# DevInsiderCoin — Start Mainnet Node
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

go build -o dvcnode ./cmd/dvcnode 2>/dev/null

./dvcnode \
  --network mainnet \
  --datadir ./data/mainnet \
  "$@"
