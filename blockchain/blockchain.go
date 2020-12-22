package blockchain

import (
	"fmt"
	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"
	lastHashKey = "lh"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB // key value store (BTC uses levelDB)
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database *badger.DB
}

func InitBlockChain() *BlockChain { // creates blockchain
	var lastHash []byte

	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(lastHashKey)); err == badger.ErrKeyNotFound {
			// no blockchain created yet so create genesis block
			fmt.Println("No existing blockchain found")
			genesis := Genesis()
			fmt.Println("Genesis proved")

			err = txn.Set(genesis.Hash, genesis.Serialize()) // set hash as key and serialized bytes as value
			Handle(err)
			err = txn.Set([]byte(lastHashKey), genesis.Hash) // keep track of last hash

			lastHash = genesis.Hash

			return err
		} else {
			item, err := txn.Get([]byte(lastHashKey))
			Handle(err)

			lastHash, err = item.Value()

			return err
		}
	})

	Handle(err)
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

func (chain *BlockChain) AddBlock(data string) {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)
		lastHash, err = item.Value()
		return err
	})
	Handle(err)

	newBlock := CreateBlock(data, lastHash)

	// add new block to DB
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize()) // add new block
		Handle(err)
		err = txn.Set([]byte(lastHashKey), newBlock.Hash) // add last hash of block

		chain.LastHash = newBlock.Hash // set last hash in memory

		return err
	})
	Handle(err)
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

func (iter *BlockChainIterator) Next() *Block { // iterate backwards until reaching genesis block
	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		Handle(err)
		encodedBlock, err := item.Value()
		block = Deserialize(encodedBlock) // deserializes and gets the current block in iterator

		return err
	})
	Handle(err)

	iter.CurrentHash = block.PrevHash

	return block
}
