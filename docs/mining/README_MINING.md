# DevInsiderCoin — Mining Guide

## Quick Start

### 1. Build

```bash
cd devinsidercoin
go build -o dvcnode.exe ./cmd/dvcnode
go build -o dvcminer.exe ./cmd/dvcminer
```

### 2. Start Node 1

```bash
dvcnode.exe --network testnet --datadir ./data/node1 --port 19333 --rpcport 19334
```

### 3. Start Node 2 (connects to Node 1)

```bash
dvcnode.exe --network testnet --datadir ./data/node2 --port 19335 --rpcport 19336 --addpeer 127.0.0.1:19333
```

### 4. Create a Wallet

```bash
curl -X POST http://localhost:19334/api/wallet/create
```

Response:
```json
{"ok": true, "data": {"address": "tDVC...", "public_key": "..."}}
```

### 5. Start Mining

```bash
dvcminer.exe --rpcaddr 127.0.0.1:19334 --address tDVC_YOUR_ADDRESS_HERE
```

### 6. Check Balance

```bash
curl "http://localhost:19334/api/wallet/balance?address=tDVC_YOUR_ADDRESS"
```

## CLI Flags

### dvcnode

| Flag | Default | Description |
|---|---|---|
| `--network` | `mainnet` | `mainnet` or `testnet` |
| `--datadir` | `./data/<network>` | Data storage directory |
| `--port` | from config | P2P port |
| `--rpcport` | from config | RPC/HTTP port |
| `--addpeer` | — | Comma-separated peer addresses |
| `--config` | — | Custom config JSON path |

### dvcminer

| Flag | Default | Description |
|---|---|---|
| `--rpcaddr` | `127.0.0.1:9334` | Node RPC address |
| `--address` | — | Mining reward address (required) |

## Block Rewards

- Initial: **250,000 DVC** per block
- **60% PoW** (miner) + **40% PoS** (stakers)
- Halving every **2,100,000 blocks** (~8 years mainnet)
- Max supply: **2^40 = 1,099,511,627,776 DVC**
- Testnet: 1-minute blocks | Mainnet: 2-minute blocks
- PoS reward threshold: **100 DVC** (mainnet) / **10 tDVC** (testnet) — below = no rewards

## Staking (PoS)

Stake coins to earn passive rewards from each block:

```bash
# Stake 100 DVC
curl -X POST http://localhost:19334/api/wallet/stake \
  -H "Content-Type: application/json" \
  -d '{"address": "DVC...", "amount": 100}'

# Unstake
curl -X POST http://localhost:19334/api/wallet/unstake \
  -H "Content-Type: application/json" \
  -d '{"address": "DVC...", "amount": 50}'
```

Minimum stake: **1,000 DVC** (mainnet) / **100 tDVC** (testnet).
