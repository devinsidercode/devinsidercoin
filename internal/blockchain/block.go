package blockchain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// BlockHeader represents the header of a block.
type BlockHeader struct {
	Version    uint32 `json:"version"`
	PrevHash   string `json:"prev_hash"`
	MerkleRoot string `json:"merkle_root"`
	Timestamp  int64  `json:"timestamp"`
	Bits       uint32 `json:"bits"`
	Nonce      uint64 `json:"nonce"`
	Height     uint64 `json:"height"`
}

// TxOutput represents a transaction output.
type TxOutput struct {
	Address string  `json:"address"`
	Amount  float64 `json:"amount"`
}

// Transaction represents a blockchain transaction.
type Transaction struct {
	TxID      string     `json:"txid"`
	Type      string     `json:"type"` // coinbase, transfer, stake, unstake, pos_reward
	From      string     `json:"from,omitempty"`
	To        string     `json:"to,omitempty"`
	Amount    float64    `json:"amount"`
	Fee       float64    `json:"fee"`
	Timestamp int64      `json:"timestamp"`
	Signature string     `json:"signature,omitempty"`
	Outputs   []TxOutput `json:"outputs,omitempty"`
}

// Block represents a full block.
type Block struct {
	Header       BlockHeader   `json:"header"`
	Transactions []Transaction `json:"transactions"`
	Hash         string        `json:"hash"`
}

// SHA256d computes double SHA-256.
func SHA256d(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

// Serialize converts the block header into bytes for hashing.
func (h *BlockHeader) Serialize() []byte {
	buf := make([]byte, 0, 128)

	b4 := make([]byte, 4)
	b8 := make([]byte, 8)

	binary.LittleEndian.PutUint32(b4, h.Version)
	buf = append(buf, b4...)

	prevHash := padHashBytes(h.PrevHash)
	buf = append(buf, prevHash...)

	merkle := padHashBytes(h.MerkleRoot)
	buf = append(buf, merkle...)

	binary.LittleEndian.PutUint64(b8, uint64(h.Timestamp))
	buf = append(buf, b8...)

	b4 = make([]byte, 4)
	binary.LittleEndian.PutUint32(b4, h.Bits)
	buf = append(buf, b4...)

	b8 = make([]byte, 8)
	binary.LittleEndian.PutUint64(b8, h.Nonce)
	buf = append(buf, b8...)

	return buf
}

func padHashBytes(hexStr string) []byte {
	decoded, _ := hex.DecodeString(hexStr)
	if len(decoded) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(decoded):], decoded)
		return padded
	}
	return decoded[:32]
}

// ComputeHash calculates the SHA-256d hash of the block header.
func (h *BlockHeader) ComputeHash() string {
	data := h.Serialize()
	hash := SHA256d(data)
	return hex.EncodeToString(hash[:])
}

// ComputeTxID computes a deterministic transaction ID.
func (tx *Transaction) ComputeTxID() string {
	data, _ := json.Marshal(struct {
		Type      string  `json:"type"`
		From      string  `json:"from"`
		To        string  `json:"to"`
		Amount    float64 `json:"amount"`
		Timestamp int64   `json:"timestamp"`
	}{tx.Type, tx.From, tx.To, tx.Amount, tx.Timestamp})
	hash := SHA256d(data)
	return hex.EncodeToString(hash[:])
}

// ComputeMerkleRoot computes a merkle root from transactions.
func ComputeMerkleRoot(txs []Transaction) string {
	if len(txs) == 0 {
		return strings.Repeat("0", 64)
	}

	hashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txData, _ := json.Marshal(tx)
		hashes[i] = SHA256d(txData)
	}

	for len(hashes) > 1 {
		var next [][32]byte
		for i := 0; i < len(hashes); i += 2 {
			var combined []byte
			combined = append(combined, hashes[i][:]...)
			if i+1 < len(hashes) {
				combined = append(combined, hashes[i+1][:]...)
			} else {
				combined = append(combined, hashes[i][:]...)
			}
			next = append(next, SHA256d(combined))
		}
		hashes = next
	}

	return hex.EncodeToString(hashes[0][:])
}

// NewCoinbaseTransaction creates a mining reward transaction.
func NewCoinbaseTransaction(minerAddress string, reward float64, height uint64) Transaction {
	tx := Transaction{
		Type:      "coinbase",
		To:        minerAddress,
		Amount:    reward,
		Timestamp: time.Now().Unix(),
		Outputs:   []TxOutput{{Address: minerAddress, Amount: reward}},
	}
	tx.TxID = tx.ComputeTxID()
	return tx
}

// NewTransferTransaction creates a transfer transaction.
func NewTransferTransaction(from, to string, amount, fee float64, sig string) Transaction {
	tx := Transaction{
		Type:      "transfer",
		From:      from,
		To:        to,
		Amount:    amount,
		Fee:       fee,
		Timestamp: time.Now().Unix(),
		Signature: sig,
	}
	tx.TxID = tx.ComputeTxID()
	return tx
}
