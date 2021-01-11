package blockchain

import (
	"github.com/stretchr/testify/assert"
	"golang-blockchain/testutils"
	"testing"
)

func TestGenesisBlockHasHeightOfZero(t *testing.T) {
	cbtx := CoinbaseTx(testutils.RandomAddress(), genesisData)
	block := Genesis(cbtx)

	assert.Equal(t, block.Height, 0)
}

func TestGenesisBlockContainsNoPreviousHash(t *testing.T) {
	cbtx := CoinbaseTx(testutils.RandomAddress(), genesisData)
	block := Genesis(cbtx)

	assert.Equal(t, len(block.PrevHash), 0)
}
