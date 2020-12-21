package blockchain

type BlockChain struct {
	Blocks []*Block
}

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

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

func (chain *BlockChain) AddBlock(data string) {
	prevBlock := chain.Blocks[len(chain.Blocks) - 1] // gets previous block
	newBlock := CreateBlock(data, prevBlock.Hash)
	chain.Blocks = append(chain.Blocks, newBlock) // returns new value
}

func Genesis() *Block { // first block in blockchain
	return CreateBlock("Genesis", []byte{})
}

func InitBlockChain() *BlockChain { // creates blockchain
	return &BlockChain{[]*Block{Genesis()}}
}
