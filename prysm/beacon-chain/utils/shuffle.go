package utils

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/beacon-chain/params"
	"golang.org/x/crypto/blake2b"
)

// ShuffleIndices returns a list of pseudorandomly sampled
// indices. This is used to use to select attesters and proposers.
func ShuffleIndices(seed common.Hash, validatorCount int) ([]int, error) {
	if validatorCount > params.MaxValidators {
		return nil, errors.New("Validator count has exceeded MaxValidator Count")
	}

	// construct a list of indices up to MaxValidators.
	validatorList := make([]int, validatorCount)
	for i := range validatorList {
		validatorList[i] = i
	}

	hashSeed := blake2b.Sum256(seed[:])

	// shuffle stops at the second to last index.
	for i := 0; i < validatorCount-1; i++ {
		// convert every 3 bytes to random number, replace validator index with that number.
		for j := 0; j+3 < len(hashSeed); j += 3 {
			swapNum := int(hashSeed[j] + hashSeed[j+1] + hashSeed[j+2])
			remaining := validatorCount - i
			swapPos := swapNum%remaining + i
			validatorList[i], validatorList[swapPos] = validatorList[swapPos], validatorList[i]
		}
	}
	return validatorList, nil
}
