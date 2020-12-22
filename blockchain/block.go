package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
)

type Block struct {
	Hash []byte // current hash
	Transactions []*Transaction // transactions on block. needs to have at least 1 transaction. can have many different transactions as well
	PrevHash []byte // last blocks hash
	Nonce int
}

// proof of work algorithm must consider transactions stored in block, instead of just hashing the previous data field
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	for _, tx := range b.Transactions { // iterate through all transactions and append to 2d slice of bytes
		txHashes = append(txHashes, tx.ID)
	}
	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{})) // concatenate and hash the bytes

	return txHash[:]
}

func CreateBlock(txs []*Transaction, prevHash []byte) *Block {
	block := &Block{[]byte{}, txs, prevHash, 0} // returns memory address of this block

	pow := NewProof(block) // create proof of work for this block
	nonce, hash := pow.Run() // try to solve the proof of work here

	block.Hash = hash[:] // set hash for the block
	block.Nonce = nonce // set solved nonce for the block. to easily validate the block

	return block
}

func Genesis(coinbase *Transaction) *Block { // first block in blockchain
	return CreateBlock([]*Transaction{coinbase}, []byte{})
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
