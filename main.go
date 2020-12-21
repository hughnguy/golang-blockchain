package main

import (
	"fmt"
	"golang-blockchain/blockchain"
	"strconv"
)

func main() {
	chain := blockchain.InitBlockChain()

	// Proof of work occurs when creating each block
	chain.AddBlock("First Block after Genesis")
	chain.AddBlock("Second Block after Genesis")
	chain.AddBlock("Third Block after Genesis")

	for _, block := range chain.Blocks { // iterate through all blocks created and validate them
		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Data in Bock: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block) // create proof and validate
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()
	}
}
