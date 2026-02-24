package blockchain

import (
	"encoding/hex"
	"math/big"
)

// CompactToBig converts compact difficulty (nBits) to a big.Int target.
func CompactToBig(compact uint32) *big.Int {
	mantissa := compact & 0x007fffff
	isNegative := (compact & 0x00800000) != 0
	exponent := compact >> 24

	var bn big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn.SetInt64(int64(mantissa))
	} else {
		bn.SetInt64(int64(mantissa))
		bn.Lsh(&bn, 8*(uint(exponent)-3))
	}
	if isNegative {
		bn.Neg(&bn)
	}
	return &bn
}

// BigToCompact converts a big.Int target to compact difficulty.
func BigToCompact(target *big.Int) uint32 {
	if target.Sign() == 0 {
		return 0
	}
	b := target.Bytes()
	size := uint32(len(b))
	var compact uint32
	if size <= 3 {
		word := uint32(0)
		for i, v := range b {
			word |= uint32(v) << (8 * (2 - uint(i)))
		}
		compact = (size << 24) | word
	} else {
		compact = (size << 24) | (uint32(b[0]) << 16) | (uint32(b[1]) << 8) | uint32(b[2])
	}
	if compact&0x00800000 != 0 {
		compact >>= 8
		size++
		compact = (size << 24) | (compact & 0x007fffff)
	}
	return compact
}

// CheckProofOfWork verifies that a block hash meets the target difficulty.
func CheckProofOfWork(hash string, bits uint32) bool {
	target := CompactToBig(bits)
	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return false
	}
	hashInt := new(big.Int).SetBytes(hashBytes)
	return hashInt.Cmp(target) <= 0
}

// CalcNextBits calculates the next difficulty target for the chain.
func CalcNextBits(blocks []*Block, cfg_interval uint64, cfg_blockTime int, cfg_minBits uint32) uint32 {
	height := uint64(len(blocks))
	if height == 0 {
		return cfg_minBits
	}
	lastBlock := blocks[len(blocks)-1]

	if height%cfg_interval != 0 {
		return lastBlock.Header.Bits
	}

	startIdx := int(height) - int(cfg_interval)
	if startIdx < 0 {
		startIdx = 0
	}
	startBlock := blocks[startIdx]

	actualTime := lastBlock.Header.Timestamp - startBlock.Header.Timestamp
	expectedTime := int64(cfg_interval) * int64(cfg_blockTime)

	if actualTime < expectedTime/4 {
		actualTime = expectedTime / 4
	}
	if actualTime > expectedTime*4 {
		actualTime = expectedTime * 4
	}

	oldTarget := CompactToBig(lastBlock.Header.Bits)
	newTarget := new(big.Int).Mul(oldTarget, big.NewInt(actualTime))
	newTarget.Div(newTarget, big.NewInt(expectedTime))

	minTarget := CompactToBig(cfg_minBits)
	if newTarget.Cmp(minTarget) > 0 {
		newTarget = minTarget
	}

	return BigToCompact(newTarget)
}
