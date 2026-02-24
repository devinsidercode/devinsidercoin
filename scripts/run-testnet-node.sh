#!/bin/bash
# DevInsiderCoin — Start Testnet Node
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

go build -o dvcnode ./cmd/dvcnode 2>/dev/null

./dvcnode \
  --network testnet \
  --datadir ./data/testnet \
  "$@"
