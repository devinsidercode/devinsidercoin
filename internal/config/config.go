package config

import (
	"encoding/json"
	"os"
)

// NetworkConfig holds all network parameters loaded from JSON manifest.
type NetworkConfig struct {
	Name                     string  `json:"name"`
	Ticker                   string  `json:"ticker"`
	NetworkID                uint32  `json:"network_id"`
	Algorithm                string  `json:"algorithm"`
	ConsensusType            string  `json:"consensus_type"`
	BlockTimeSeconds         int     `json:"block_time_seconds"`
	InitialReward            float64 `json:"initial_reward"`
	POWRewardShare           float64 `json:"pow_reward_share"`
	POSRewardShare           float64 `json:"pos_reward_share"`
	HalvingInterval          uint64  `json:"halving_interval"`
	MaxSupply                float64 `json:"max_supply"`
	DifficultyAdjustInterval uint64  `json:"difficulty_adjustment_interval"`
	MinDifficultyBits        uint32  `json:"min_difficulty_bits"`
	GenesisTimestamp         string  `json:"genesis_timestamp"`
	GenesisMessage           string  `json:"genesis_message"`
	P2PPort                  int     `json:"p2p_port"`
	RPCPort                  int     `json:"rpc_port"`
	AddressPrefix            string  `json:"address_prefix"`
	ProtocolVersion          uint32  `json:"protocol_version"`
	MinStakeAmount           float64 `json:"min_stake_amount"`
	StakeLockBlocks          uint64  `json:"stake_lock_blocks"`
	MaxBlockSize             uint64  `json:"max_block_size"`
	MaxBlockTransactions     uint64  `json:"max_block_transactions"`
	POSMinThreshold          float64 `json:"pos_min_threshold"`
	DifficultyEpochBlocks    uint64  `json:"difficulty_epoch_blocks"`
}

// LoadConfig reads a network configuration from a JSON file.
func LoadConfig(path string) (*NetworkConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg NetworkConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Defaults for backward compatibility
	if cfg.MaxBlockSize == 0 {
		cfg.MaxBlockSize = 8 * 1024 * 1024 // 8 MB
	}
	if cfg.MaxBlockTransactions == 0 {
		cfg.MaxBlockTransactions = 10000
	}
	if cfg.POSMinThreshold == 0 {
		cfg.POSMinThreshold = 100.0
	}
	if cfg.DifficultyEpochBlocks == 0 {
		cfg.DifficultyEpochBlocks = 500000
	}
	return &cfg, nil
}
