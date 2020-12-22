package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
)

type Block struct {
	Hash []byte // current hash
	Data []byte // data on block
	PrevHash []byte // last blocks hash
	Nonce int
}

func CreateBlock(data string, prevHash []byte) *Block {
	block := &Block{[]byte{}, []byte(data), prevHash, 0} // returns memory address of this block

	pow := NewProof(block) // create proof of work for this block
	nonce, hash := pow.Run() // try to solve the proof of work here

	block.Hash = hash[:] // set hash for the block
	block.Nonce = nonce // set solved nonce for the block. to easily validate the block

	return block
}

func Genesis() *Block { // first block in blockchain
	return CreateBlock("Genesis", []byte{})
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
