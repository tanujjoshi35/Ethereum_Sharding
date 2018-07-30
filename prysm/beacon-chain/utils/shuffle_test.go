package utils

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/beacon-chain/params"
)

func TestFaultyShuffleIndices(t *testing.T) {
	if _, err := ShuffleIndices(common.Hash{'a'}, params.MaxValidators+1); err == nil {
		t.Error("Shuffle should have failed when validator count exceeds MaxValidators")
	}
}

func TestShuffleIndices(t *testing.T) {
	hash1 := common.BytesToHash([]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'a', 'b', 'c', 'd', 'e', 'f', 'g'})
	hash2 := common.BytesToHash([]byte{'1', '2', '3', '4', '5', '6', '7', '1', '2', '3', '4', '5', '6', '7', '1', '2', '3', '4', '5', '6', '7', '1', '2', '3', '4', '5', '6', '7', '1', '2', '3', '4', '5', '6', '7'})

	list1, err := ShuffleIndices(hash1, 100)
	if err != nil {
		t.Errorf("Shuffle failed with: %v", err)
	}

	list2, err := ShuffleIndices(hash2, 100)
	if err != nil {
		t.Errorf("Shuffle failed with: %v", err)
	}
	if reflect.DeepEqual(list1, list2) {
		t.Errorf("2 shuffled lists shouldn't be equal")
	}
}
