package blockchain

import (
	"fmt"
	"sync"
)

// Stake represents a user's staked coins.
type Stake struct {
	Address     string  `json:"address"`
	Amount      float64 `json:"amount"`
	BlockHeight uint64  `json:"block_height"`
}

// StakeManager tracks all active stakes.
type StakeManager struct {
	Stakes map[string]*Stake `json:"stakes"`
	mu     sync.RWMutex
}

// NewStakeManager creates a new stake manager.
func NewStakeManager() *StakeManager {
	return &StakeManager{Stakes: make(map[string]*Stake)}
}

// AddStake adds or increases stake for an address.
func (sm *StakeManager) AddStake(address string, amount float64, height uint64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.Stakes[address]; ok {
		s.Amount += amount
	} else {
		sm.Stakes[address] = &Stake{Address: address, Amount: amount, BlockHeight: height}
	}
}

// RemoveStake removes stake for an address.
func (sm *StakeManager) RemoveStake(address string, amount float64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.Stakes[address]
	if !ok {
		return fmt.Errorf("no stake found for %s", address)
	}
	if s.Amount < amount {
		return fmt.Errorf("insufficient stake: have %.8f, want %.8f", s.Amount, amount)
	}
	s.Amount -= amount
	if s.Amount < 0.00000001 {
		delete(sm.Stakes, address)
	}
	return nil
}

// GetTotalStaked returns total staked coins across all addresses.
func (sm *StakeManager) GetTotalStaked() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	total := 0.0
	for _, s := range sm.Stakes {
		total += s.Amount
	}
	return total
}

// GetStake returns the staked amount for an address.
func (sm *StakeManager) GetStake(address string) float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if s, ok := sm.Stakes[address]; ok {
		return s.Amount
	}
	return 0
}

// CalcPOSRewards distributes PoS reward proportionally among stakers
// whose stake is at or above minThreshold. Stakers below the threshold
// are excluded from rewards entirely.
func (sm *StakeManager) CalcPOSRewards(totalReward float64, minThreshold float64) []TxOutput {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Calculate total eligible stake (only stakers >= minThreshold).
	eligibleStaked := 0.0
	for _, s := range sm.Stakes {
		if s.Amount >= minThreshold {
			eligibleStaked += s.Amount
		}
	}
	if eligibleStaked == 0 {
		return nil
	}

	var outputs []TxOutput
	for addr, s := range sm.Stakes {
		if s.Amount < minThreshold {
			continue // below threshold — no rewards
		}
		share := s.Amount / eligibleStaked
		reward := totalReward * share
		if reward > 0.00000001 {
			outputs = append(outputs, TxOutput{Address: addr, Amount: reward})
		}
	}
	return outputs
}

// GetAllStakes returns a copy of all stakes.
func (sm *StakeManager) GetAllStakes() map[string]*Stake {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	cp := make(map[string]*Stake, len(sm.Stakes))
	for k, v := range sm.Stakes {
		s := *v
		cp[k] = &s
	}
	return cp
}
