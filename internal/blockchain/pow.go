package blockchain

import (
	"encoding/hex"
	"math/big"
)

// BitsToTarget converts compact "bits" representation to a 256-bit target.
// Format: 0xNNBBBBBB where NN = exponent, BBBBBB = mantissa.
func BitsToTarget(bits uint32) *big.Int {
	exponent := bits >> 24
	mantissa := bits & 0x007FFFFF
	target := new(big.Int).SetUint64(uint64(mantissa))
	if exponent <= 3 {
		target.Rsh(target, uint(8*(3-exponent)))
	} else {
		target.Lsh(target, uint(8*(exponent-3)))
	}
	return target
}

// TargetToBits converts a 256-bit target back to compact "bits" format.
func TargetToBits(target *big.Int) uint32 {
	if target.Sign() == 0 {
		return 0
	}
	bytes := target.Bytes()
	exponent := uint32(len(bytes))
	var mantissa uint32
	if len(bytes) >= 3 {
		mantissa = uint32(bytes[0])<<16 | uint32(bytes[1])<<8 | uint32(bytes[2])
	} else if len(bytes) == 2 {
		mantissa = uint32(bytes[0])<<8 | uint32(bytes[1])
	} else {
		mantissa = uint32(bytes[0])
	}
	// If the high bit is set, shift right to avoid negative interpretation.
	if mantissa&0x800000 != 0 {
		mantissa >>= 8
		exponent++
	}
	return (exponent << 24) | mantissa
}

// CheckProofOfWork checks if a block hash satisfies the difficulty target.
func CheckProofOfWork(hashHex string, bits uint32) bool {
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil || len(hashBytes) != 32 {
		return false
	}
	hashInt := new(big.Int).SetBytes(hashBytes)
	target := BitsToTarget(bits)
	return hashInt.Cmp(target) <= 0
}

// CalcNextBits performs standard difficulty retargeting with bounds, used every
// DifficultyAdjustInterval blocks.  The result is then clamped by the
// progressive difficulty floor for the current height.
func CalcNextBits(blocks []*Block, adjustInterval uint64, targetSeconds int, minBits uint32) uint32 {
	n := len(blocks)
	if n == 0 {
		return minBits
	}

	// Need at least adjustInterval blocks.
	if uint64(n) < adjustInterval {
		return blocks[n-1].Header.Bits
	}

	last := blocks[n-1]
	first := blocks[n-int(adjustInterval)]

	actualTime := last.Header.Timestamp - first.Header.Timestamp
	expectedTime := int64(adjustInterval) * int64(targetSeconds)

	// Clamp adjustment to 4x range (no more than 4x easier or harder).
	if actualTime < expectedTime/4 {
		actualTime = expectedTime / 4
	}
	if actualTime > expectedTime*4 {
		actualTime = expectedTime * 4
	}

	currentTarget := BitsToTarget(last.Header.Bits)
	newTarget := new(big.Int).Mul(currentTarget, big.NewInt(actualTime))
	newTarget.Div(newTarget, big.NewInt(expectedTime))

	// Don't exceed the maximum target (minimum difficulty).
	maxTarget := BitsToTarget(minBits)
	if newTarget.Cmp(maxTarget) > 0 {
		newTarget.Set(maxTarget)
	}

	return TargetToBits(newTarget)
}

// ProgressiveDifficultyFloor returns the minimum bits (maximum target) allowed
// at a given height.  Every DifficultyEpochBlocks blocks the floor tightens by
// halving the max target, making mining progressively harder over time.
//
// Epoch 0 (blocks 0 – epoch-1):       floor = minBits (easiest)
// Epoch 1 (blocks epoch – 2*epoch-1): floor target = minTarget / 2
// Epoch 2:                             floor target = minTarget / 4
// ...
//
// This guarantees that even if hash rate drops, the network never becomes
// trivially easy to mine at high block heights.
func ProgressiveDifficultyFloor(height uint64, epochBlocks uint64, minBits uint32) uint32 {
	if epochBlocks == 0 {
		return minBits
	}
	epoch := height / epochBlocks
	if epoch == 0 {
		return minBits
	}
	// Cap epoch shifts to prevent target from becoming zero.
	if epoch > 60 {
		epoch = 60
	}
	maxTarget := BitsToTarget(minBits)
	floorTarget := new(big.Int).Rsh(maxTarget, uint(epoch)) // divide by 2^epoch
	if floorTarget.Sign() == 0 {
		floorTarget.SetInt64(1)
	}
	return TargetToBits(floorTarget)
}

// ApplyProgressiveDifficulty clamps bits so they never exceed the progressive
// floor for the given height.  "Exceed" means the target is too large (mining
// too easy).  Smaller bits = harder mining.
func ApplyProgressiveDifficulty(bits uint32, height uint64, epochBlocks uint64, minBits uint32) uint32 {
	floorBits := ProgressiveDifficultyFloor(height, epochBlocks, minBits)
	// A higher target value = easier mining.  We want target <= floor target.
	target := BitsToTarget(bits)
	floorTarget := BitsToTarget(floorBits)
	if target.Cmp(floorTarget) > 0 {
		return floorBits
	}
	return bits
}
