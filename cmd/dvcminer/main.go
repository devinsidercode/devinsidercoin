package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"time"
)

type BlockHeader struct {
	Version    uint32 `json:"version"`
	PrevHash   string `json:"prev_hash"`
	MerkleRoot string `json:"merkle_root"`
	Timestamp  int64  `json:"timestamp"`
	Bits       uint32 `json:"bits"`
	Nonce      uint64 `json:"nonce"`
	Height     uint64 `json:"height"`
}

type Block struct {
	Header       BlockHeader     `json:"header"`
	Transactions json.RawMessage `json:"transactions"`
	Hash         string          `json:"hash"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  interface{}     `json:"error"`
	ID     interface{}     `json:"id"`
}

func main() {
	rpcAddr := flag.String("rpcaddr", "127.0.0.1:9334", "Node RPC address (host:port)")
	minerAddr := flag.String("address", "", "Mining reward address")
	flag.Parse()

	if *minerAddr == "" {
		log.Fatal("Mining address required. Use -address <your_dvc_address>")
	}

	log.Printf("=== DevInsiderCoin Miner ===")
	log.Printf("  RPC:     %s", *rpcAddr)
	log.Printf("  Address: %s", *minerAddr)

	rpcURL := fmt.Sprintf("http://%s/rpc", *rpcAddr)
	totalMined := 0

	for {
		tmpl, err := getBlockTemplate(rpcURL, *minerAddr)
		if err != nil {
			log.Printf("[MINER] Error getting template: %v (retrying in 5s)", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("[MINER] Mining block #%d (bits: 0x%08x)...",
			tmpl.Header.Height, tmpl.Header.Bits)

		startTime := time.Now()

		for nonce := uint64(0); nonce < ^uint64(0); nonce++ {
			tmpl.Header.Nonce = nonce
			hash := computeHash(&tmpl.Header)

			if nonce%500000 == 0 && nonce > 0 {
				elapsed := time.Since(startTime).Seconds()
				rate := uint64(float64(nonce) / elapsed)
				fmt.Printf("\r  Hashrate: %d H/s | Nonce: %d", rate, nonce)
			}

			if checkPoW(hash, tmpl.Header.Bits) {
				tmpl.Hash = hash
				elapsed := time.Since(startTime)
				fmt.Println()
				log.Printf("[MINER] ✓ Block #%d found! Hash: %s (%.2fs, nonce: %d)",
					tmpl.Header.Height, hash[:16]+"...", elapsed.Seconds(), nonce)

				if err := submitBlock(rpcURL, tmpl); err != nil {
					log.Printf("[MINER] Submit error: %v", err)
				} else {
					totalMined++
					log.Printf("[MINER] Block accepted! Total mined: %d", totalMined)
				}
				break
			}
		}
	}
}

func computeHash(h *BlockHeader) string {
	buf := make([]byte, 0, 128)
	buf = appendU32(buf, h.Version)
	buf = append(buf, decodeHexPad(h.PrevHash, 32)...)
	buf = append(buf, decodeHexPad(h.MerkleRoot, 32)...)
	buf = appendU64(buf, uint64(h.Timestamp))
	buf = appendU32(buf, h.Bits)
	buf = appendU64(buf, h.Nonce)
	first := sha256.Sum256(buf)
	second := sha256.Sum256(first[:])
	return hex.EncodeToString(second[:])
}

func checkPoW(hash string, bits uint32) bool {
	target := compactToBig(bits)
	hashBytes, _ := hex.DecodeString(hash)
	hashInt := new(big.Int).SetBytes(hashBytes)
	return hashInt.Cmp(target) <= 0
}

func compactToBig(compact uint32) *big.Int {
	mantissa := compact & 0x007fffff
	exponent := compact >> 24
	var bn big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn.SetInt64(int64(mantissa))
	} else {
		bn.SetInt64(int64(mantissa))
		bn.Lsh(&bn, 8*(uint(exponent)-3))
	}
	return &bn
}

func decodeHexPad(s string, size int) []byte {
	b, _ := hex.DecodeString(s)
	if len(b) < size {
		pad := make([]byte, size)
		copy(pad[size-len(b):], b)
		return pad
	}
	return b[:size]
}

func appendU32(buf []byte, v uint32) []byte {
	return append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func appendU64(buf []byte, v uint64) []byte {
	return append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func getBlockTemplate(rpcURL, addr string) (*Block, error) {
	params, _ := json.Marshal(map[string]string{"miner_address": addr})
	reqBody, _ := json.Marshal(map[string]interface{}{
		"method": "getblocktemplate", "params": json.RawMessage(params), "id": 1,
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var rr RPCResponse
	json.Unmarshal(body, &rr)
	if rr.Error != nil {
		return nil, fmt.Errorf("%v", rr.Error)
	}
	var block Block
	json.Unmarshal(rr.Result, &block)
	return &block, nil
}

func submitBlock(rpcURL string, block *Block) error {
	blockJSON, _ := json.Marshal(block)
	reqBody, _ := json.Marshal(map[string]interface{}{
		"method": "submitblock", "params": json.RawMessage(blockJSON), "id": 2,
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var rr RPCResponse
	json.Unmarshal(body, &rr)
	if rr.Error != nil {
		return fmt.Errorf("%v", rr.Error)
	}
	return nil
}
