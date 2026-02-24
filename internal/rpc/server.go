package rpc

import (
	"devinsidercoin/internal/blockchain"
	"devinsidercoin/internal/network"
	"devinsidercoin/internal/wallet"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Server handles JSON-RPC (mining) and REST (wallet) HTTP endpoints.
type Server struct {
	Chain   *blockchain.Blockchain
	Node    *network.Node
	Wallets *wallet.WalletManager
	Addr    string
}

// JSONRPCRequest is the incoming JSON-RPC format.
type JSONRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     interface{}     `json:"id"`
}

// JSONRPCResponse is the outgoing JSON-RPC format.
type JSONRPCResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
	ID     interface{} `json:"id"`
}

// Start begins the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// JSON-RPC endpoint (mining)
	mux.HandleFunc("/rpc", s.handleRPC)

	// REST wallet API
	mux.HandleFunc("/api/wallet/create", s.handleWalletCreate)
	mux.HandleFunc("/api/wallet/list", s.handleWalletList)
	mux.HandleFunc("/api/wallet/backup", s.handleWalletBackup)
	mux.HandleFunc("/api/wallet/restore", s.handleWalletRestore)
	mux.HandleFunc("/api/wallet/send", s.handleWalletSend)
	mux.HandleFunc("/api/wallet/balance", s.handleWalletBalance)
	mux.HandleFunc("/api/wallet/transactions", s.handleWalletTransactions)
	mux.HandleFunc("/api/wallet/stake", s.handleWalletStake)
	mux.HandleFunc("/api/wallet/unstake", s.handleWalletUnstake)

	// Chain info API
	mux.HandleFunc("/api/chain/info", s.handleChainInfo)
	mux.HandleFunc("/api/chain/block", s.handleChainBlock)

	log.Printf("[RPC] HTTP server listening on %s", s.Addr)
	return http.ListenAndServe(s.Addr, withCORS(mux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": data})
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": msg})
}

// ========== JSON-RPC (Mining) ==========

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, "parse error")
		return
	}

	switch req.Method {
	case "getblocktemplate":
		s.rpcGetBlockTemplate(w, req)
	case "submitblock":
		s.rpcSubmitBlock(w, req)
	case "getblockcount":
		writeRPCResult(w, req.ID, s.Chain.GetBlockCount())
	case "getbestblockhash":
		best := s.Chain.GetBestBlock()
		if best != nil {
			writeRPCResult(w, req.ID, best.Hash)
		} else {
			writeRPCResult(w, req.ID, "")
		}
	case "getmininginfo":
		best := s.Chain.GetBestBlock()
		bits := uint32(0)
		if best != nil {
			bits = best.Header.Bits
		}
		writeRPCResult(w, req.ID, map[string]interface{}{
			"blocks":       s.Chain.GetBlockCount(),
			"difficulty":   bits,
			"network_hash": 0,
			"staked_total": s.Chain.Stakes.GetTotalStaked(),
			"mempool_size": len(s.Chain.GetMempool()),
			"peers":        s.Node.GetPeerCount(),
		})
	case "getpeerinfo":
		writeRPCResult(w, req.ID, s.Node.GetPeerAddresses())
	default:
		writeRPCError(w, req.ID, "unknown method: "+req.Method)
	}
}

func (s *Server) rpcGetBlockTemplate(w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		MinerAddress string `json:"miner_address"`
	}
	json.Unmarshal(req.Params, &params)
	if params.MinerAddress == "" {
		writeRPCError(w, req.ID, "miner_address required")
		return
	}
	tmpl := s.Chain.CreateBlockTemplate(params.MinerAddress)
	writeRPCResult(w, req.ID, tmpl)
}

func (s *Server) rpcSubmitBlock(w http.ResponseWriter, req JSONRPCRequest) {
	var block blockchain.Block
	if err := json.Unmarshal(req.Params, &block); err != nil {
		writeRPCError(w, req.ID, "invalid block: "+err.Error())
		return
	}
	if err := s.Chain.AddBlock(&block); err != nil {
		writeRPCError(w, req.ID, err.Error())
		return
	}
	// Broadcast to peers
	s.Node.BroadcastBlock(&block)
	writeRPCResult(w, req.ID, map[string]interface{}{
		"accepted": true,
		"hash":     block.Hash,
		"height":   block.Header.Height,
	})
}

func writeRPCResult(w http.ResponseWriter, id interface{}, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JSONRPCResponse{Result: result, ID: id})
}

func writeRPCError(w http.ResponseWriter, id interface{}, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JSONRPCResponse{Error: msg, ID: id})
}

// ========== REST Wallet API ==========

func (s *Server) handleWalletCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, 405, "POST required")
		return
	}
	wlt, err := s.Wallets.CreateWallet()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{
		"address":    wlt.Address,
		"public_key": wlt.PublicKey,
	})
}

func (s *Server) handleWalletList(w http.ResponseWriter, r *http.Request) {
	addrs := s.Wallets.ListWallets()
	type walletInfo struct {
		Address string  `json:"address"`
		Balance float64 `json:"balance"`
		Staked  float64 `json:"staked"`
	}
	var list []walletInfo
	for _, addr := range addrs {
		list = append(list, walletInfo{
			Address: addr,
			Balance: s.Chain.GetBalance(addr),
			Staked:  s.Chain.Stakes.GetStake(addr),
		})
	}
	jsonOK(w, list)
}

func (s *Server) handleWalletBackup(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		jsonErr(w, 400, "address parameter required")
		return
	}
	data, err := s.Wallets.Backup(address)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_backup.json", address))
	w.Write(data)
}

func (s *Server) handleWalletRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, 405, "POST required")
		return
	}
	body, _ := io.ReadAll(r.Body)
	wlt, err := s.Wallets.Restore(body)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonOK(w, map[string]string{"address": wlt.Address, "status": "restored"})
}

func (s *Server) handleWalletSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, 405, "POST required")
		return
	}
	var req struct {
		From   string  `json:"from"`
		To     string  `json:"to"`
		Amount float64 `json:"amount"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.From == "" || req.To == "" || req.Amount <= 0 {
		jsonErr(w, 400, "from, to, and amount (>0) required")
		return
	}

	// Sign the transaction
	txData := fmt.Sprintf("%s:%s:%.8f:%d", req.From, req.To, req.Amount, time.Now().Unix())
	sig, err := s.Wallets.Sign(req.From, []byte(txData))
	if err != nil {
		jsonErr(w, 400, "cannot sign: "+err.Error())
		return
	}

	fee := 0.001
	tx := blockchain.NewTransferTransaction(req.From, req.To, req.Amount, fee, sig)

	if err := s.Chain.AddToMempool(tx); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	// Broadcast to peers
	s.Node.BroadcastTx(&tx)

	jsonOK(w, map[string]interface{}{
		"txid":   tx.TxID,
		"from":   tx.From,
		"to":     tx.To,
		"amount": tx.Amount,
		"fee":    tx.Fee,
		"status": "pending",
	})
}

func (s *Server) handleWalletBalance(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		jsonErr(w, 400, "address parameter required")
		return
	}
	balance := s.Chain.GetBalance(address)
	staked := s.Chain.Stakes.GetStake(address)
	jsonOK(w, map[string]interface{}{
		"address":   address,
		"balance":   balance,
		"staked":    staked,
		"available": balance - staked,
	})
}

func (s *Server) handleWalletTransactions(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		jsonErr(w, 400, "address parameter required")
		return
	}
	txs := s.Chain.GetTransactions(address)
	jsonOK(w, txs)
}

func (s *Server) handleWalletStake(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, 405, "POST required")
		return
	}
	var req struct {
		Address string  `json:"address"`
		Amount  float64 `json:"amount"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)
	if req.Address == "" || req.Amount <= 0 {
		jsonErr(w, 400, "address and amount (>0) required")
		return
	}

	tx := blockchain.Transaction{
		Type:      "stake",
		From:      req.Address,
		Amount:    req.Amount,
		Timestamp: time.Now().Unix(),
	}
	tx.TxID = tx.ComputeTxID()

	if err := s.Chain.AddToMempool(tx); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	s.Node.BroadcastTx(&tx)
	jsonOK(w, map[string]interface{}{"txid": tx.TxID, "status": "pending"})
}

func (s *Server) handleWalletUnstake(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, 405, "POST required")
		return
	}
	var req struct {
		Address string  `json:"address"`
		Amount  float64 `json:"amount"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)
	if req.Address == "" || req.Amount <= 0 {
		jsonErr(w, 400, "address and amount (>0) required")
		return
	}

	tx := blockchain.Transaction{
		Type:      "unstake",
		From:      req.Address,
		Amount:    req.Amount,
		Timestamp: time.Now().Unix(),
	}
	tx.TxID = tx.ComputeTxID()

	if err := s.Chain.AddToMempool(tx); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	s.Node.BroadcastTx(&tx)
	jsonOK(w, map[string]interface{}{"txid": tx.TxID, "status": "pending"})
}

// ========== Chain Info API ==========

func (s *Server) handleChainInfo(w http.ResponseWriter, r *http.Request) {
	best := s.Chain.GetBestBlock()
	hash := ""
	bits := uint32(0)
	if best != nil {
		hash = best.Hash
		bits = best.Header.Bits
	}
	jsonOK(w, map[string]interface{}{
		"name":         s.Chain.Config.Name,
		"ticker":       s.Chain.Config.Ticker,
		"blocks":       s.Chain.GetBlockCount(),
		"best_hash":    hash,
		"difficulty":   bits,
		"staked_total": s.Chain.Stakes.GetTotalStaked(),
		"mempool_size": len(s.Chain.GetMempool()),
		"peers":        s.Node.GetPeerCount(),
	})
}

func (s *Server) handleChainBlock(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash != "" {
		block := s.Chain.GetBlockByHash(hash)
		if block == nil {
			jsonErr(w, 404, "block not found")
			return
		}
		jsonOK(w, block)
		return
	}
	jsonErr(w, 400, "hash parameter required")
}
