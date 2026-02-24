# DevInsiderCoin — Genesis Block

## Parameters

| Field | Value |
|---|---|
| **Timestamp** | 2026-02-24T12:00:00Z (Mainnet) / 2026-02-24T00:00:00Z (Testnet) |
| **Message** | "DevInsiderCoin Genesis - Internal Company Currency 2026" |
| **Height** | 0 |
| **Previous Hash** | 0000000000000000000000000000000000000000000000000000000000000000 |
| **Bits** | 0x1F00FFFF (starting difficulty) |
| **Nonce** | 0 |
| **Coinbase Reward** | 0 DVC (genesis block has no reward) |

## How Genesis is Created

The genesis block is created deterministically from the network config file (`networks/mainnet.json` or `networks/testnet.json`). The genesis hash is computed via SHA-256d (double SHA-256) of the serialized block header.

Key function: `blockchain.CreateGenesisBlock(config)` in `internal/blockchain/genesis.go`.

## Consensus: Hybrid PoW + PoS

Starting from block #1:
- **60%** of block reward → PoW miner (who found the block)
- **40%** of block reward → PoS stakers (proportional to stake)
- If no stakers exist, PoW miner receives 100% of the reward
- Initial block reward: **50 DVC**
- Halving every **210,000 blocks**
