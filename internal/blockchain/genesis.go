package blockchain

import (
	"devinsidercoin/internal/config"
	"strings"
	"time"
)

// CreateGenesisBlock creates the genesis (first) block for the network.
func CreateGenesisBlock(cfg *config.NetworkConfig) *Block {
	ts, err := time.Parse(time.RFC3339, cfg.GenesisTimestamp)
	if err != nil {
		ts = time.Now()
	}

	coinbase := Transaction{
		Type:      "coinbase",
		To:        "genesis",
		Amount:    0,
		Timestamp: ts.Unix(),
		Outputs:   []TxOutput{{Address: "genesis", Amount: 0}},
	}
	coinbase.TxID = coinbase.ComputeTxID()

	merkle := ComputeMerkleRoot([]Transaction{coinbase})

	header := BlockHeader{
		Version:    1,
		PrevHash:   strings.Repeat("0", 64),
		MerkleRoot: merkle,
		Timestamp:  ts.Unix(),
		Bits:       cfg.MinDifficultyBits,
		Nonce:      0,
		Height:     0,
	}

	block := &Block{
		Header:       header,
		Transactions: []Transaction{coinbase},
	}
	block.Hash = header.ComputeHash()

	return block
}
