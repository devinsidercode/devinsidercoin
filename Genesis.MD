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

## Tokenomics

| Parameter | Value |
|---|---|
| **Max Supply** | 2^40 = 1,099,511,627,776 DVC |
| **Initial Block Reward** | 250,000 DVC |
| **Halving Interval** | 2,100,000 blocks (~8 years) |
| **Theoretical Emission** | 250,000 × 2,100,000 × 2 ≈ 1.05T (hard-capped at 2^40) |

## Consensus: Hybrid PoW + PoS

Starting from block #1:
- **60%** of block reward → PoW miner (who found the block)
- **40%** of block reward → PoS stakers (proportional to stake)
- If no eligible stakers exist, PoW miner receives 100% of the reward
- Minimum stake: **1,000 DVC** (mainnet)
- PoS reward threshold: **100 DVC** — stakers below this receive nothing

## Progressive Mining Difficulty

The network implements a progressive difficulty floor that increases over time:

- Every **500,000 blocks** (~1.9 years), the minimum allowed difficulty doubles
- This ensures mining becomes progressively harder regardless of hash rate fluctuations
- The standard difficulty retarget (every 72 blocks) still operates, but cannot drop below the progressive floor
- Epoch 0: base difficulty | Epoch 1: 2× harder | Epoch 2: 4× harder | Epoch N: 2^N × harder

## Block Limits

| Parameter | Value |
|---|---|
| **Max Block Size** | 8 MB (8,388,608 bytes) |
| **Max Transactions per Block** | 10,000 |