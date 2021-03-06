package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
	"time"
)

type Block struct {
	Timestamp int64
	Hash []byte // current hash
	Transactions []*Transaction // transactions on block. needs to have at least 1 transaction. can have many different transactions as well
	PrevHash []byte // last blocks hash
	Nonce int
	Height int
}

// proof of work algorithm must consider transactions stored in block, instead of just hashing the previous data field
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte

	for _, tx := range b.Transactions { // iterate through all transactions and append to 2d slice of bytes
		txHashes = append(txHashes, tx.Serialize())
	}
	// generate a merkle tree using all transactions on this block. this can be used to quickly detect if a transaction belongs to a block??
	tree := NewMerkleTree(txHashes)

	// returns the merkle root
	return tree.RootNode.Data
}

func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{time.Now().Unix(), []byte{}, txs, prevHash, 0, height} // returns memory address of this block

	pow := NewProof(block) // create proof of work for this block
	nonce, hash := pow.Run() // try to solve the proof of work here

	block.Hash = hash[:] // set hash for the block
	block.Nonce = nonce // set solved nonce for the block. to easily validate the block

	return block
}

func Genesis(coinbase *Transaction) *Block { // first block in blockchain
	return CreateBlock([]*Transaction{coinbase}, []byte{}, 0) // 0 since first block in blockchain
}

// serialize for database storage
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)

	Handle(err)

	return res.Bytes()
}

// deserialize when grabbing bytes from database
func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)

	Handle(err)

	return &block
}

func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
