package storage

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

// Bucket names.
var (
	bucketBlocks    = []byte("blocks")       // height (8 bytes BE) -> JSON block
	bucketBlockHash = []byte("block_hashes") // hash -> height (8 bytes BE)
	bucketBalances  = []byte("balances")     // address -> JSON float
	bucketStakes    = []byte("stakes")       // address -> JSON stake
	bucketTxIndex   = []byte("tx_index")     // txid -> height (8 bytes BE)
	bucketMeta      = []byte("meta")         // key -> value
)

var (
	metaBestHeight  = []byte("best_height")
	metaTotalMinted = []byte("total_minted")
)

// Store wraps BoltDB for blockchain persistence.
type Store struct {
	db   *bolt.DB
	Path string
}

// NewStore opens or creates a BoltDB database.
func NewStore(dataDir string) (*Store, error) {
	os.MkdirAll(dataDir, 0755)
	dbPath := filepath.Join(dataDir, "blockchain.db")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketBlocks, bucketBlockHash, bucketBalances,
			bucketStakes, bucketTxIndex, bucketMeta,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db, Path: dbPath}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func heightKey(h uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, h)
	return b
}

func keyToHeight(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func floatToBytes(f float64) []byte {
	b, _ := json.Marshal(f)
	return b
}

func bytesToFloat(b []byte) float64 {
	var f float64
	json.Unmarshal(b, &f)
	return f
}

// --- Block operations ---

func (s *Store) GetBestHeight() int64 {
	var h int64 = -1
	s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get(metaBestHeight)
		if v != nil {
			h = int64(keyToHeight(v))
		}
		return nil
	})
	return h
}

func (s *Store) GetBlockCount() uint64 {
	h := s.GetBestHeight()
	if h < 0 {
		return 0
	}
	return uint64(h) + 1
}

func (s *Store) HasData() bool {
	return s.GetBestHeight() >= 0
}

func (s *Store) GetBlockByHeight(height uint64) ([]byte, error) {
	var data []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketBlocks).Get(heightKey(height))
		if v == nil {
			return fmt.Errorf("block not found at height %d", height)
		}
		data = make([]byte, len(v))
		copy(data, v)
		return nil
	})
	return data, err
}

func (s *Store) GetBlockByHash(hash string) ([]byte, error) {
	var data []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		hk := tx.Bucket(bucketBlockHash).Get([]byte(hash))
		if hk == nil {
			return fmt.Errorf("block not found: %s", hash)
		}
		v := tx.Bucket(bucketBlocks).Get(hk)
		if v == nil {
			return fmt.Errorf("block data missing for hash %s", hash)
		}
		data = make([]byte, len(v))
		copy(data, v)
		return nil
	})
	return data, err
}

func (s *Store) GetBlocksFrom(startHeight uint64) ([][]byte, error) {
	var blocks [][]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		c := b.Cursor()
		for k, v := c.Seek(heightKey(startHeight)); k != nil; k, v = c.Next() {
			data := make([]byte, len(v))
			copy(data, v)
			blocks = append(blocks, data)
		}
		return nil
	})
	return blocks, err
}

// GetRecentBlocks returns the last N blocks (for difficulty calculation).
func (s *Store) GetRecentBlocks(count uint64) ([][]byte, error) {
	best := s.GetBestHeight()
	if best < 0 {
		return nil, nil
	}
	start := uint64(0)
	if uint64(best) >= count {
		start = uint64(best) - count + 1
	}
	var blocks [][]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		for h := start; h <= uint64(best); h++ {
			v := b.Get(heightKey(h))
			if v == nil {
				continue
			}
			data := make([]byte, len(v))
			copy(data, v)
			blocks = append(blocks, data)
		}
		return nil
	})
	return blocks, err
}

// --- Balance ---

func (s *Store) GetBalance(address string) float64 {
	var bal float64
	s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketBalances).Get([]byte(address))
		if v != nil {
			bal = bytesToFloat(v)
		}
		return nil
	})
	return bal
}

func (s *Store) GetAllBalances() map[string]float64 {
	balances := make(map[string]float64)
	s.db.View(func(tx *bolt.Tx) error {
		tx.Bucket(bucketBalances).ForEach(func(k, v []byte) error {
			balances[string(k)] = bytesToFloat(v)
			return nil
		})
		return nil
	})
	return balances
}

// --- Stakes ---

func (s *Store) GetAllStakesRaw() map[string][]byte {
	stakes := make(map[string][]byte)
	s.db.View(func(tx *bolt.Tx) error {
		tx.Bucket(bucketStakes).ForEach(func(k, v []byte) error {
			data := make([]byte, len(v))
			copy(data, v)
			stakes[string(k)] = data
			return nil
		})
		return nil
	})
	return stakes
}

// --- TX Index ---

func (s *Store) GetTxBlockHeight(txid string) (uint64, error) {
	var height uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketTxIndex).Get([]byte(txid))
		if v == nil {
			return fmt.Errorf("tx not found: %s", txid)
		}
		height = keyToHeight(v)
		return nil
	})
	return height, err
}

// --- Meta ---

func (s *Store) GetTotalMinted() float64 {
	var total float64
	s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get(metaTotalMinted)
		if v != nil {
			total = bytesToFloat(v)
		}
		return nil
	})
	return total
}

// --- Atomic block commit ---

// BlockCommit holds all state changes for a new block.
type BlockCommit struct {
	Height      uint64
	Hash        string
	BlockJSON   []byte
	Balances    map[string]float64 // address -> new balance
	Stakes      map[string][]byte  // address -> JSON stake (nil = delete)
	TxIDs       []string
	TotalMinted float64
}

// CommitBlock atomically writes all changes for a new block.
func (s *Store) CommitBlock(c *BlockCommit) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		hk := heightKey(c.Height)

		if err := tx.Bucket(bucketBlocks).Put(hk, c.BlockJSON); err != nil {
			return err
		}
		if err := tx.Bucket(bucketBlockHash).Put([]byte(c.Hash), hk); err != nil {
			return err
		}

		bb := tx.Bucket(bucketBalances)
		for addr, bal := range c.Balances {
			if err := bb.Put([]byte(addr), floatToBytes(bal)); err != nil {
				return err
			}
		}

		sb := tx.Bucket(bucketStakes)
		for addr, data := range c.Stakes {
			if data == nil {
				sb.Delete([]byte(addr))
			} else {
				if err := sb.Put([]byte(addr), data); err != nil {
					return err
				}
			}
		}

		tb := tx.Bucket(bucketTxIndex)
		for _, txid := range c.TxIDs {
			if err := tb.Put([]byte(txid), hk); err != nil {
				return err
			}
		}

		if err := tx.Bucket(bucketMeta).Put(metaBestHeight, hk); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put(metaTotalMinted, floatToBytes(c.TotalMinted))
	})
}
