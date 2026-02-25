package blockchain

import (
	"devinsidercoin/internal/config"
	"devinsidercoin/internal/storage"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Blockchain manages the chain state.
type Blockchain struct {
	Config      *config.NetworkConfig
	Store       *storage.Store
	Balances    map[string]float64
	Stakes      *StakeManager
	Mempool     []Transaction
	TotalMinted float64
	DataDir     string
	mu          sync.RWMutex
	lastBlock   *Block
}

// NewBlockchain creates or loads a blockchain.
func NewBlockchain(cfg *config.NetworkConfig, dataDir string) *Blockchain {
	store, err := storage.NewStore(dataDir)
	if err != nil {
		log.Fatalf("[CHAIN] Failed to open database: %v", err)
	}

	bc := &Blockchain{
		Config:   cfg,
		Store:    store,
		Balances: make(map[string]float64),
		Stakes:   NewStakeManager(),
		Mempool:  make([]Transaction, 0),
		DataDir:  dataDir,
	}

	if !store.HasData() {
		if bc.migrateFromJSON() {
			log.Printf("[CHAIN] Migrated from blockchain.json to BoltDB")
		} else {
			genesis := CreateGenesisBlock(cfg)
			blockJSON, _ := json.Marshal(genesis)
			commit := &storage.BlockCommit{
				Height:      0,
				Hash:        genesis.Hash,
				BlockJSON:   blockJSON,
				Balances:    bc.Balances,
				TxIDs:       collectTxIDs(genesis),
				TotalMinted: 0,
			}
			if err := store.CommitBlock(commit); err != nil {
				log.Fatalf("[CHAIN] Failed to write genesis: %v", err)
			}
			bc.lastBlock = genesis
			log.Printf("[CHAIN] Created genesis block: %s", genesis.Hash)
		}
	} else {
		bc.Balances = store.GetAllBalances()
		bc.TotalMinted = store.GetTotalMinted()
		bc.loadStakesFromDB()
		bc.lastBlock = bc.loadBlock(uint64(store.GetBestHeight()))
		log.Printf("[CHAIN] Loaded %d blocks from BoltDB (minted: %.2f / %.2f)",
			store.GetBlockCount(), bc.TotalMinted, cfg.MaxSupply)
	}

	return bc
}

func (bc *Blockchain) Close() {
	if bc.Store != nil {
		bc.Store.Close()
	}
}

func collectTxIDs(block *Block) []string {
	ids := make([]string, len(block.Transactions))
	for i, tx := range block.Transactions {
		ids[i] = tx.TxID
	}
	return ids
}

func (bc *Blockchain) loadBlock(height uint64) *Block {
	data, err := bc.Store.GetBlockByHeight(height)
	if err != nil {
		return nil
	}
	var block Block
	if err := json.Unmarshal(data, &block); err != nil {
		return nil
	}
	return &block
}

func (bc *Blockchain) loadStakesFromDB() {
	raw := bc.Store.GetAllStakesRaw()
	for addr, data := range raw {
		var s Stake
		if json.Unmarshal(data, &s) == nil {
			bc.Stakes.Stakes[addr] = &s
		}
	}
}

// --- Migration from old JSON format ---

type oldChainData struct {
	Blocks      []*Block           `json:"blocks"`
	Balances    map[string]float64 `json:"balances"`
	Stakes      map[string]*Stake  `json:"stakes"`
	TotalMinted float64            `json:"total_minted"`
}

func (bc *Blockchain) migrateFromJSON() bool {
	jsonPath := filepath.Join(bc.DataDir, "blockchain.json")
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return false
	}
	var data oldChainData
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	if len(data.Blocks) == 0 {
		return false
	}

	log.Printf("[CHAIN] Migrating %d blocks from JSON to BoltDB...", len(data.Blocks))

	for i, block := range data.Blocks {
		blockJSON, _ := json.Marshal(block)
		commit := &storage.BlockCommit{
			Height:    block.Header.Height,
			Hash:      block.Hash,
			BlockJSON: blockJSON,
			TxIDs:     collectTxIDs(block),
		}
		// On last block, include all state.
		if i == len(data.Blocks)-1 {
			commit.Balances = data.Balances
			commit.TotalMinted = data.TotalMinted
			if data.Stakes != nil {
				stakeMap := make(map[string][]byte)
				for addr, s := range data.Stakes {
					sJSON, _ := json.Marshal(s)
					stakeMap[addr] = sJSON
				}
				commit.Stakes = stakeMap
			}
		}
		if err := bc.Store.CommitBlock(commit); err != nil {
			log.Printf("[CHAIN] Migration error at block %d: %v", block.Header.Height, err)
			return false
		}
	}

	if data.Balances != nil {
		bc.Balances = data.Balances
	}
	if data.Stakes != nil {
		for addr, s := range data.Stakes {
			bc.Stakes.Stakes[addr] = s
		}
	}
	bc.TotalMinted = data.TotalMinted
	bc.lastBlock = data.Blocks[len(data.Blocks)-1]

	backupPath := jsonPath + ".migrated"
	os.Rename(jsonPath, backupPath)
	log.Printf("[CHAIN] Old blockchain.json renamed to %s", backupPath)
	return true
}

// --- Public API ---

func (bc *Blockchain) GetBestHeight() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	h := bc.Store.GetBestHeight()
	if h < 0 {
		return 0
	}
	return uint64(h)
}

func (bc *Blockchain) GetBestBlock() *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.lastBlock
}

func (bc *Blockchain) GetBlockByHeight(height uint64) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.loadBlock(height)
}

func (bc *Blockchain) GetBlockByHash(hash string) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	data, err := bc.Store.GetBlockByHash(hash)
	if err != nil {
		return nil
	}
	var block Block
	json.Unmarshal(data, &block)
	return &block
}

func (bc *Blockchain) GetBalance(address string) float64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.Balances[address]
}

func (bc *Blockchain) GetTransactions(address string) []Transaction {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	var result []Transaction
	count := bc.Store.GetBlockCount()
	for h := uint64(0); h < count; h++ {
		block := bc.loadBlock(h)
		if block == nil {
			continue
		}
		for _, tx := range block.Transactions {
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

func (bc *Blockchain) GetBlockCount() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.Store.GetBlockCount()
}

func (bc *Blockchain) GetTotalMinted() float64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.TotalMinted
}

func (bc *Blockchain) CalcBlockReward(height uint64) float64 {
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
	remaining := bc.Config.MaxSupply - bc.TotalMinted
	if reward > remaining {
		reward = remaining
	}
	return reward
}

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
		totalStake := bc.Stakes.GetStake(tx.From) + tx.Amount
		if totalStake < bc.Config.POSMinThreshold {
			return fmt.Errorf("total stake must be at least %.2f %s to participate in PoS",
				bc.Config.POSMinThreshold, bc.Config.Ticker)
		}
	}
	bc.Mempool = append(bc.Mempool, tx)
	return nil
}

func (bc *Blockchain) GetMempool() []Transaction {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	cp := make([]Transaction, len(bc.Mempool))
	copy(cp, bc.Mempool)
	return cp
}

func (bc *Blockchain) CreateBlockTemplate(minerAddress string) *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	height := bc.Store.GetBlockCount()
	prevHash := strings.Repeat("0", 64)
	prevBits := bc.Config.MinDifficultyBits

	if bc.lastBlock != nil && height > 0 {
		prevHash = bc.lastBlock.Hash
		prevBits = bc.lastBlock.Header.Bits
	}

	totalReward := bc.CalcBlockReward(height)
	powReward := totalReward * bc.Config.POWRewardShare
	posReward := totalReward * bc.Config.POSRewardShare

	var txs []Transaction
	posOutputs := bc.Stakes.CalcPOSRewards(posReward, bc.Config.POSMinThreshold)
	if len(posOutputs) > 0 {
		txs = append(txs, NewCoinbaseTransaction(minerAddress, powReward, height))
		posTx := Transaction{
			Type:      "pos_reward",
			Amount:    posReward,
			Timestamp: time.Now().Unix(),
			Outputs:   posOutputs,
		}
		posTx.TxID = posTx.ComputeTxID()
		txs = append(txs, posTx)
	} else {
		txs = append(txs, NewCoinbaseTransaction(minerAddress, totalReward, height))
	}

	maxTxs := int(bc.Config.MaxBlockTransactions) - len(txs)
	if maxTxs > len(bc.Mempool) {
		maxTxs = len(bc.Mempool)
	}
	if maxTxs > 0 {
		txs = append(txs, bc.Mempool[:maxTxs]...)
	}

	bits := prevBits
	if height > 0 && height%bc.Config.DifficultyAdjustInterval == 0 {
		bits = bc.calcNextBitsFromDB()
	}
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

func (bc *Blockchain) calcNextBitsFromDB() uint32 {
	interval := bc.Config.DifficultyAdjustInterval
	rawBlocks, err := bc.Store.GetRecentBlocks(interval)
	if err != nil || uint64(len(rawBlocks)) < interval {
		if bc.lastBlock != nil {
			return bc.lastBlock.Header.Bits
		}
		return bc.Config.MinDifficultyBits
	}
	blocks := make([]*Block, len(rawBlocks))
	for i, raw := range rawBlocks {
		var b Block
		json.Unmarshal(raw, &b)
		blocks[i] = &b
	}
	return CalcNextBits(blocks, interval,
		bc.Config.BlockTimeSeconds, bc.Config.MinDifficultyBits)
}

func (bc *Blockchain) AddBlock(block *Block) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if err := bc.validateBlock(block); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	changedBalances := make(map[string]float64)
	changedStakes := make(map[string][]byte)
	var blockMinted float64

	for _, tx := range block.Transactions {
		switch tx.Type {
		case "coinbase":
			for _, out := range tx.Outputs {
				bc.Balances[out.Address] += out.Amount
				changedBalances[out.Address] = bc.Balances[out.Address]
				blockMinted += out.Amount
			}
		case "pos_reward":
			for _, out := range tx.Outputs {
				bc.Balances[out.Address] += out.Amount
				changedBalances[out.Address] = bc.Balances[out.Address]
				blockMinted += out.Amount
			}
		case "transfer":
			bc.Balances[tx.From] -= tx.Amount + tx.Fee
			bc.Balances[tx.To] += tx.Amount
			changedBalances[tx.From] = bc.Balances[tx.From]
			changedBalances[tx.To] = bc.Balances[tx.To]
		case "stake":
			bc.Balances[tx.From] -= tx.Amount
			changedBalances[tx.From] = bc.Balances[tx.From]
			bc.Stakes.AddStake(tx.From, tx.Amount, block.Header.Height)
			sJSON, _ := json.Marshal(bc.Stakes.Stakes[tx.From])
			changedStakes[tx.From] = sJSON
		case "unstake":
			bc.Stakes.RemoveStake(tx.From, tx.Amount)
			bc.Balances[tx.From] += tx.Amount
			changedBalances[tx.From] = bc.Balances[tx.From]
			if s, ok := bc.Stakes.Stakes[tx.From]; ok {
				sJSON, _ := json.Marshal(s)
				changedStakes[tx.From] = sJSON
			} else {
				changedStakes[tx.From] = nil
			}
		}
	}

	bc.TotalMinted += blockMinted

	blockJSON, _ := json.Marshal(block)
	commit := &storage.BlockCommit{
		Height:      block.Header.Height,
		Hash:        block.Hash,
		BlockJSON:   blockJSON,
		Balances:    changedBalances,
		Stakes:      changedStakes,
		TxIDs:       collectTxIDs(block),
		TotalMinted: bc.TotalMinted,
	}
	if err := bc.Store.CommitBlock(commit); err != nil {
		return fmt.Errorf("db commit failed: %w", err)
	}

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
	bc.lastBlock = block

	log.Printf("[CHAIN] Block #%d added: %s (txs: %d, minted: %.2f, total: %.2f/%.2f)",
		block.Header.Height, block.Hash[:16]+"...", len(block.Transactions),
		blockMinted, bc.TotalMinted, bc.Config.MaxSupply)
	return nil
}

func (bc *Blockchain) validateBlock(block *Block) error {
	expectedHeight := bc.Store.GetBlockCount()
	if block.Header.Height != expectedHeight {
		return fmt.Errorf("bad height: expected %d, got %d", expectedHeight, block.Header.Height)
	}
	if expectedHeight > 0 && bc.lastBlock != nil {
		if block.Header.PrevHash != bc.lastBlock.Hash {
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
	if uint64(len(block.Transactions)) > bc.Config.MaxBlockTransactions {
		return fmt.Errorf("too many transactions: %d > %d",
			len(block.Transactions), bc.Config.MaxBlockTransactions)
	}
	blockData, _ := json.Marshal(block)
	if uint64(len(blockData)) > bc.Config.MaxBlockSize {
		return fmt.Errorf("block too large: %d bytes > %d",
			len(blockData), bc.Config.MaxBlockSize)
	}
	floorBits := ProgressiveDifficultyFloor(block.Header.Height,
		bc.Config.DifficultyEpochBlocks, bc.Config.MinDifficultyBits)
	blockTarget := BitsToTarget(block.Header.Bits)
	floorTarget := BitsToTarget(floorBits)
	if blockTarget.Cmp(floorTarget) > 0 {
		return fmt.Errorf("difficulty below progressive floor at height %d", block.Header.Height)
	}
	return nil
}

func (bc *Blockchain) GetBlocks(startHeight uint64) []*Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	rawBlocks, err := bc.Store.GetBlocksFrom(startHeight)
	if err != nil {
		return nil
	}
	blocks := make([]*Block, 0, len(rawBlocks))
	for _, raw := range rawBlocks {
		var b Block
		if json.Unmarshal(raw, &b) == nil {
			blocks = append(blocks, &b)
		}
	}
	return blocks
}
