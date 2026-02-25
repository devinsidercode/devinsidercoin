package blockchain

import (
	"devinsidercoin/internal/config"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ChainData is the persistent on-disk format.
type ChainData struct {
	Blocks      []*Block           `json:"blocks"`
	Balances    map[string]float64 `json:"balances"`
	Stakes      map[string]*Stake  `json:"stakes"`
	TotalMinted float64            `json:"total_minted"`
}

// Blockchain manages the chain state.
type Blockchain struct {
	Config      *config.NetworkConfig
	Blocks      []*Block
	Balances    map[string]float64
	Stakes      *StakeManager
	Mempool     []Transaction
	TxIndex     map[string]*Transaction // txid -> tx
	TotalMinted float64                 // total coins ever minted
	DataDir     string
	mu          sync.RWMutex
}

// NewBlockchain creates or loads a blockchain.
func NewBlockchain(cfg *config.NetworkConfig, dataDir string) *Blockchain {
	bc := &Blockchain{
		Config:   cfg,
		Blocks:   make([]*Block, 0),
		Balances: make(map[string]float64),
		Stakes:   NewStakeManager(),
		Mempool:  make([]Transaction, 0),
		TxIndex:  make(map[string]*Transaction),
		DataDir:  dataDir,
	}
	if err := bc.loadFromDisk(); err != nil {
		genesis := CreateGenesisBlock(cfg)
		bc.Blocks = append(bc.Blocks, genesis)
		bc.indexBlockTxs(genesis)
		bc.saveToDisk()
		log.Printf("[CHAIN] Created genesis block: %s", genesis.Hash)
	} else {
		log.Printf("[CHAIN] Loaded %d blocks from disk (minted: %.2f / %.2f)",
			len(bc.Blocks), bc.TotalMinted, cfg.MaxSupply)
	}
	return bc
}

func (bc *Blockchain) indexBlockTxs(block *Block) {
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		bc.TxIndex[tx.TxID] = tx
	}
}

// GetBestHeight returns the height of the latest block.
func (bc *Blockchain) GetBestHeight() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if len(bc.Blocks) == 0 {
		return 0
	}
	return bc.Blocks[len(bc.Blocks)-1].Header.Height
}

// GetBestBlock returns the latest block.
func (bc *Blockchain) GetBestBlock() *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if len(bc.Blocks) == 0 {
		return nil
	}
	return bc.Blocks[len(bc.Blocks)-1]
}

// GetBlockByHeight returns a block at the given height.
func (bc *Blockchain) GetBlockByHeight(height uint64) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if height >= uint64(len(bc.Blocks)) {
		return nil
	}
	return bc.Blocks[height]
}

// GetBlockByHash returns a block by hash.
func (bc *Blockchain) GetBlockByHash(hash string) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	for _, b := range bc.Blocks {
		if b.Hash == hash {
			return b
		}
	}
	return nil
}

// GetBalance returns the balance of an address.
func (bc *Blockchain) GetBalance(address string) float64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.Balances[address]
}

// GetTransactions returns all transactions involving an address.
func (bc *Blockchain) GetTransactions(address string) []Transaction {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	var result []Transaction
	for _, b := range bc.Blocks {
		for _, tx := range b.Transactions {
			if tx.From == address || tx.To == address {
				result = append(result, tx)
				continue
			}
			for _, out := range tx.Outputs {
				if out.Address == address {
					result = append(result, tx)
					break
				}
			}
		}
	}
	return result
}

// GetBlockCount returns total number of blocks.
func (bc *Blockchain) GetBlockCount() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return uint64(len(bc.Blocks))
}

// GetTotalMinted returns total coins minted so far.
func (bc *Blockchain) GetTotalMinted() float64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.TotalMinted
}

// CalcBlockReward returns the block reward at given height, enforcing max supply.
func (bc *Blockchain) CalcBlockReward(height uint64) float64 {
	// All coins minted — no more rewards.
	if bc.TotalMinted >= bc.Config.MaxSupply {
		return 0
	}

	halvings := height / bc.Config.HalvingInterval
	reward := bc.Config.InitialReward
	for i := uint64(0); i < halvings; i++ {
		reward /= 2
	}
	if reward < 0.00000001 {
		return 0
	}

	// Cap reward so we never exceed max supply.
	remaining := bc.Config.MaxSupply - bc.TotalMinted
	if reward > remaining {
		reward = remaining
	}

	return reward
}

// AddToMempool adds a transaction to the mempool after validation.
func (bc *Blockchain) AddToMempool(tx Transaction) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if tx.Type == "transfer" {
		if bc.Balances[tx.From] < tx.Amount+tx.Fee {
			return fmt.Errorf("insufficient balance: have %.8f, need %.8f",
				bc.Balances[tx.From], tx.Amount+tx.Fee)
		}
	}
	if tx.Type == "stake" {
		available := bc.Balances[tx.From] - bc.Stakes.GetStake(tx.From)
		if available < tx.Amount {
			return fmt.Errorf("insufficient available balance for staking")
		}
		if tx.Amount < bc.Config.MinStakeAmount {
			return fmt.Errorf("minimum stake is %.2f %s", bc.Config.MinStakeAmount, bc.Config.Ticker)
		}
		// Also check that total stake after this tx meets the threshold.
		totalStake := bc.Stakes.GetStake(tx.From) + tx.Amount
		if totalStake < bc.Config.POSMinThreshold {
			return fmt.Errorf("total stake must be at least %.2f %s to participate in PoS",
				bc.Config.POSMinThreshold, bc.Config.Ticker)
		}
	}
	bc.Mempool = append(bc.Mempool, tx)
	return nil
}

// GetMempool returns a copy of the mempool.
func (bc *Blockchain) GetMempool() []Transaction {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	cp := make([]Transaction, len(bc.Mempool))
	copy(cp, bc.Mempool)
	return cp
}

// CreateBlockTemplate builds a new block ready for mining.
func (bc *Blockchain) CreateBlockTemplate(minerAddress string) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	height := uint64(len(bc.Blocks))
	prevHash := strings.Repeat("0", 64)
	prevBits := bc.Config.MinDifficultyBits

	if len(bc.Blocks) > 0 {
		last := bc.Blocks[len(bc.Blocks)-1]
		prevHash = last.Hash
		prevBits = last.Header.Bits
	}

	totalReward := bc.CalcBlockReward(height)
	powReward := totalReward * bc.Config.POWRewardShare
	posReward := totalReward * bc.Config.POSRewardShare

	var txs []Transaction

	// PoS rewards — only stakers above POSMinThreshold.
	posOutputs := bc.Stakes.CalcPOSRewards(posReward, bc.Config.POSMinThreshold)
	if len(posOutputs) > 0 {
		coinbase := NewCoinbaseTransaction(minerAddress, powReward, height)
		txs = append(txs, coinbase)
		posTx := Transaction{
			Type:      "pos_reward",
			Amount:    posReward,
			Timestamp: time.Now().Unix(),
			Outputs:   posOutputs,
		}
		posTx.TxID = posTx.ComputeTxID()
		txs = append(txs, posTx)
	} else {
		coinbase := NewCoinbaseTransaction(minerAddress, totalReward, height)
		txs = append(txs, coinbase)
	}

	// Add mempool txs (up to max block transactions).
	maxTxs := int(bc.Config.MaxBlockTransactions) - len(txs)
	if maxTxs > len(bc.Mempool) {
		maxTxs = len(bc.Mempool)
	}
	if maxTxs > 0 {
		txs = append(txs, bc.Mempool[:maxTxs]...)
	}

	// Difficulty retarget.
	bits := prevBits
	if height > 0 && height%bc.Config.DifficultyAdjustInterval == 0 {
		bits = CalcNextBits(bc.Blocks, bc.Config.DifficultyAdjustInterval,
			bc.Config.BlockTimeSeconds, bc.Config.MinDifficultyBits)
	}

	// Apply progressive difficulty floor.
	bits = ApplyProgressiveDifficulty(bits, height,
		bc.Config.DifficultyEpochBlocks, bc.Config.MinDifficultyBits)

	merkle := ComputeMerkleRoot(txs)

	header := BlockHeader{
		Version:    2,
		PrevHash:   prevHash,
		MerkleRoot: merkle,
		Timestamp:  time.Now().Unix(),
		Bits:       bits,
		Nonce:      0,
		Height:     height,
	}

	return &Block{Header: header, Transactions: txs}
}

// AddBlock validates and adds a block to the chain.
func (bc *Blockchain) AddBlock(block *Block) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if err := bc.validateBlock(block); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Track minted coins in this block.
	var blockMinted float64

	// Apply transactions.
	for _, tx := range block.Transactions {
		switch tx.Type {
		case "coinbase":
			for _, out := range tx.Outputs {
				bc.Balances[out.Address] += out.Amount
				blockMinted += out.Amount
			}
		case "pos_reward":
			for _, out := range tx.Outputs {
				bc.Balances[out.Address] += out.Amount
				blockMinted += out.Amount
			}
		case "transfer":
			bc.Balances[tx.From] -= tx.Amount + tx.Fee
			bc.Balances[tx.To] += tx.Amount
		case "stake":
			bc.Balances[tx.From] -= tx.Amount
			bc.Stakes.AddStake(tx.From, tx.Amount, block.Header.Height)
		case "unstake":
			bc.Stakes.RemoveStake(tx.From, tx.Amount)
			bc.Balances[tx.From] += tx.Amount
		}
	}

	bc.TotalMinted += blockMinted

	// Clear processed txs from mempool.
	processed := make(map[string]bool)
	for _, tx := range block.Transactions {
		processed[tx.TxID] = true
	}
	var remaining []Transaction
	for _, tx := range bc.Mempool {
		if !processed[tx.TxID] {
			remaining = append(remaining, tx)
		}
	}
	bc.Mempool = remaining

	bc.Blocks = append(bc.Blocks, block)
	bc.indexBlockTxs(block)
	bc.saveToDisk()

	log.Printf("[CHAIN] Block #%d added: %s (txs: %d, minted: %.2f, total: %.2f/%.2f)",
		block.Header.Height, block.Hash[:16]+"...", len(block.Transactions),
		blockMinted, bc.TotalMinted, bc.Config.MaxSupply)
	return nil
}

func (bc *Blockchain) validateBlock(block *Block) error {
	expectedHeight := uint64(len(bc.Blocks))
	if block.Header.Height != expectedHeight {
		return fmt.Errorf("bad height: expected %d, got %d", expectedHeight, block.Header.Height)
	}
	if expectedHeight > 0 {
		prev := bc.Blocks[len(bc.Blocks)-1]
		if block.Header.PrevHash != prev.Hash {
			return fmt.Errorf("bad prev hash")
		}
	}
	computed := block.Header.ComputeHash()
	if block.Hash != computed {
		return fmt.Errorf("bad hash: computed %s, got %s", computed, block.Hash)
	}
	if !CheckProofOfWork(block.Hash, block.Header.Bits) {
		return fmt.Errorf("insufficient proof of work")
	}
	// Block size limits.
	if uint64(len(block.Transactions)) > bc.Config.MaxBlockTransactions {
		return fmt.Errorf("too many transactions: %d > %d",
			len(block.Transactions), bc.Config.MaxBlockTransactions)
	}
	blockData, _ := json.Marshal(block)
	if uint64(len(blockData)) > bc.Config.MaxBlockSize {
		return fmt.Errorf("block too large: %d bytes > %d",
			len(blockData), bc.Config.MaxBlockSize)
	}
	// Validate progressive difficulty floor.
	floorBits := ProgressiveDifficultyFloor(block.Header.Height,
		bc.Config.DifficultyEpochBlocks, bc.Config.MinDifficultyBits)
	blockTarget := BitsToTarget(block.Header.Bits)
	floorTarget := BitsToTarget(floorBits)
	if blockTarget.Cmp(floorTarget) > 0 {
		return fmt.Errorf("difficulty below progressive floor at height %d", block.Header.Height)
	}
	return nil
}

// GetBlocks returns blocks from startHeight to the tip.
func (bc *Blockchain) GetBlocks(startHeight uint64) []*Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if startHeight >= uint64(len(bc.Blocks)) {
		return nil
	}
	result := make([]*Block, len(bc.Blocks)-int(startHeight))
	copy(result, bc.Blocks[startHeight:])
	return result
}

func (bc *Blockchain) saveToDisk() {
	os.MkdirAll(bc.DataDir, 0755)
	data := ChainData{
		Blocks:      bc.Blocks,
		Balances:    bc.Balances,
		Stakes:      bc.Stakes.GetAllStakes(),
		TotalMinted: bc.TotalMinted,
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(bc.DataDir, "blockchain.json"), raw, 0644)
}

func (bc *Blockchain) loadFromDisk() error {
	raw, err := os.ReadFile(filepath.Join(bc.DataDir, "blockchain.json"))
	if err != nil {
		return err
	}
	var data ChainData
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if len(data.Blocks) == 0 {
		return fmt.Errorf("empty chain data")
	}
	bc.Blocks = data.Blocks
	bc.Balances = data.Balances
	bc.TotalMinted = data.TotalMinted
	if data.Stakes != nil {
		for k, v := range data.Stakes {
			bc.Stakes.Stakes[k] = v
		}
	}
	for _, b := range bc.Blocks {
		bc.indexBlockTxs(b)
	}
	return nil
}
