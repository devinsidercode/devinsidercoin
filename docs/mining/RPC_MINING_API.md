# DevInsiderCoin — RPC & HTTP API Reference

Base URL: `http://localhost:9334` (mainnet) or `http://localhost:19334` (testnet)

---

## JSON-RPC (Mining) — `POST /rpc`

### getblocktemplate
Get a block template for mining.
```json
{"method": "getblocktemplate", "params": {"miner_address": "DVC..."}, "id": 1}
```

### submitblock
Submit a mined block.
```json
{"method": "submitblock", "params": {<block object>}, "id": 2}
```

### getblockcount
```json
{"method": "getblockcount", "params": null, "id": 3}
```
Returns: `{"result": 42}`

### getbestblockhash
```json
{"method": "getbestblockhash", "params": null, "id": 4}
```

### getmininginfo
```json
{"method": "getmininginfo", "params": null, "id": 5}
```
Returns: blocks, difficulty, staked_total, mempool_size, peers

### getpeerinfo
```json
{"method": "getpeerinfo", "params": null, "id": 6}
```

---

## REST Wallet API

All responses: `{"ok": true/false, "data": ..., "error": "..."}`

### POST /api/wallet/create
Creates a new wallet.
```json
// Response
{"ok": true, "data": {"address": "DVC...", "public_key": "..."}}
```

### GET /api/wallet/list
Lists all wallets on this node with balances.

### GET /api/wallet/backup?address=DVC...
Downloads wallet backup JSON (includes private key).

### POST /api/wallet/restore
Restores a wallet from backup JSON.
```json
// Body: wallet backup JSON
{"address": "DVC...", "public_key": "...", "private_key": "..."}
```

### POST /api/wallet/send
Send coins.
```json
{"from": "DVC_sender", "to": "DVC_receiver", "amount": 10.5}
```
Response includes txid, fee (0.001 DVC), status "pending".

### GET /api/wallet/balance?address=DVC...
```json
{"ok": true, "data": {"address": "DVC...", "balance": 150.0, "staked": 50.0, "available": 100.0}}
```

### GET /api/wallet/transactions?address=DVC...
Returns all transactions for the address.

### POST /api/wallet/stake
```json
{"address": "DVC...", "amount": 100.0}
```

### POST /api/wallet/unstake
```json
{"address": "DVC...", "amount": 50.0}
```

---

## Chain Info API

### GET /api/chain/info
Returns network name, ticker, block count, best hash, difficulty, staked total, mempool size, peers.

### GET /api/chain/block?hash=abc...
Returns full block data by hash.

---

## CORS

All endpoints support CORS (`Access-Control-Allow-Origin: *`) for frontend integration.
