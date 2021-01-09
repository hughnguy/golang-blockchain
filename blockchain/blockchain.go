package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dbPath = "./tmp/blocks_%s"
)

const (
	genesisData = "First Transaction from Genesis"
	lastHashKey = "lh"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB // key value store (BTC uses levelDB)
}

// MANIFEST used to verify whether or not instance of blockchain database exists
// path is the database location for a specific node
func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}

func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	// attempt to open database. it can throw an error if the DB has corrupt data (process exited prematurely)
	if db, err := badger.Open(opts); err != nil {

		// check if LOCK file exists. this happens if process exits prematurely and corrupts data
		if strings.Contains(err.Error(), "LOCK") {
			// attempts to unlock the database that was LOCKED (due to process exiting prematurely)
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			// could not fix corrupt data
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		// database is fine, so return it
		return db, nil
	}
}

// unlocks the database that was LOCKED (due to process exiting prematurely and causing corrupt data)
func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	// get path to lock
	lockPath := filepath.Join(dir, "LOCK")

	// remove the lock
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	// use original options
	retryOpts := originalOpts
	// Truncate value log to delete corrupt data, if any.
	// recovers database if one of our processes exits prematurely
	retryOpts.Truncate = true

	// retries opening database
	db, err := badger.Open(retryOpts)
	return db, err
}

func InitBlockChain(address, nodeId string) *BlockChain { // creates blockchain
	var lastHash []byte

	path := fmt.Sprintf(dbPath, nodeId)

	if DBexists(path) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := openDB(path, opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		// no blockchain created yet so create genesis block
		cbtx := CoinbaseTx(address, genesisData) // address will be rewarded tokens
		genesis := Genesis(cbtx)
		fmt.Println("Genesis block created")
		err = txn.Set(genesis.Hash, genesis.Serialize()) // set hash as key and serialized bytes as value
		Handle(err)
		err = txn.Set([]byte(lastHashKey), genesis.Hash) // keep track of last hash

		lastHash = genesis.Hash

		return err
	})

	Handle(err)
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

func ContinueBlockChain(nodeId string) *BlockChain {
	var lastHash []byte

	// create database path for specific node
	path := fmt.Sprintf(dbPath, nodeId)

	// check if db exists for this node
	if DBexists(path) == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	// opens database and cleans up any corrupt data
	db, err := openDB(path, opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)

		lastHash, err = item.Value()

		return err
	})
	Handle(err)

	chain := BlockChain{lastHash, db}
	return &chain
}

// adds new block if it doesnt exist yet, and sets it
// as the last hash key if the block is a newer version than our latest block
func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {
		// if block already exists, dont need to add so do nothing
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}
		// serialize block and store block in database using block hash as key
		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		Handle(err)

		// get last block hash from database
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)
		lastHash, _ := item.Value()

		// get and serialize the last block
		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.Value()

		lastBlock := Deserialize(lastBlockData)

		// if the block we just added is a newer one than our last block,
		// then set this new block's hash as our last hash key
		if block.Height > lastBlock.Height {
			err := txn.Set([]byte(lastHashKey), block.Hash)
			Handle(err)
			// set as last hash on our chain
			chain.LastHash = block.Hash
		}

		return nil
	})
	Handle(err)
}

func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		// error if block does not exist
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block is not found")
		} else {
			// otherwise deserialize block
			blockData, _ := item.Value()
			block = *Deserialize(blockData)
		}
		return nil
	})
	if err != nil {
		return block, err
	}
	return block, nil
}

func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blockHashes [][]byte

	iter := chain.Iterator()

	// keep iterating until reaching first genesis block (no previous hash)
	for {
		block := iter.Next()

		blockHashes = append(blockHashes, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return blockHashes
}

// returns height of the last block
func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		// get hash of last block
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)
		lastHash, _ := item.Value()

		// get last block and deserialize
		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.Value()
		lastBlock = *Deserialize(lastBlockData)

		return nil
	})

	Handle(err)

	// return height of last block
	return lastBlock.Height
}

func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	// I don't think this is necessary since we already verify all transactions in the MineTx() function that calls MineBlock()
	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}

	err := chain.Database.View(func(txn *badger.Txn) error {
		// get and assign last hash
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)
		lastHash, err = item.Value()

		// get last block
		item, err = txn.Get(lastHash)
		Handle(err)

		// deserialize last block
		lastBlockData, _ := item.Value()
		lastBlock := Deserialize(lastBlockData)

		// assign the height for this last block that was mined
		lastHeight = lastBlock.Height

		return err
	})
	Handle(err)

	// increment the height when creating new block. also pass in previous blocks hash
	newBlock := CreateBlock(transactions, lastHash, lastHeight + 1)

	// add new block to DB
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize()) // add new block
		Handle(err)
		err = txn.Set([]byte(lastHashKey), newBlock.Hash) // add last hash of block

		chain.LastHash = newBlock.Hash // set last hash in memory

		return err
	})
	Handle(err)

	return newBlock
}

// Gets unspent transactions for this particular public key hash
func (chain *BlockChain) FindUnspentTransactions(pubKeyHash []byte) []Transaction {
	// transactions which contain outputs that have not been used for another transactions inputs (a transaction that has your UTXOs)
	var unspentTxs []Transaction

	// contains a map of your transactions that have spent outputs (outputs used in another transactions inputs)
	// key is transaction ID where outputs belong
	// value is array of indexes for outputs
	spentTXOs := make(map[string][]int) // map where keys are strings and value is array of ints

	iter := chain.Iterator()

	for { // iterate through all blocks
		block := iter.Next()

		// iterate through all transactions on each block
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs: // this is a label so that we can break/continue out of this outer loop and not the inner loop
			for outIdx, out := range tx.Outputs { // iterate through all outputs of this transaction
				// outIdx is index of output in array

				// if the transaction contains some of your spent outputs (spent output = output that ends up being in another input therefore spent??)
				if spentTXOs[txID] != nil {

					// then iterate through all your spent outputs
					for _, spentOutIdx := range spentTXOs[txID] {
						// if the current output (outIdx) is one of your spent outputs in this transaction, skip to next output.
						// we only want to find outputs that have not been spent yet
						if spentOutIdx == outIdx {
							continue Outputs // skip to next output
						}
					}
				}
				// otherwise, reaching here means this transaction's output has not been spent yet
				// and if the output was sent to you (locked with your address) and is one of your unspent ones, add transaction to array (since this transaction contains some of your UTXOs)
				if out.IsLockedWithKey(pubKeyHash) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}
			// iterate through all inputs for this transaction
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					// if this input uses this public key hash, it means you sent money to someone (input has you as the sender)
					// so this input was previously another transactions output, therefore the output is now spent
					if in.UsesKey(pubKeyHash) {
						// get the ID of the previous transaction that this input was in (when it was an output)
						inTxID := hex.EncodeToString(in.ID)
						// add the output index (when the input was previously an output) to the map
						// this marks your output as spent for that transaction
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		if len(block.PrevHash) == 0 { // break if on genesis block
			break
		}
	}
	return unspentTxs
}

// Gets ALL unspent transaction outputs (for the entire blockchain, not just yours)
func (chain *BlockChain) FindAllUTXOs() map[string]TxOutputs {
	// unspent outputs that have not been used for another transaction's inputs
	// key contains transaction ID that output belongs to
	UTXO := make(map[string]TxOutputs)
	// contains a map of any transactions that have spent outputs (outputs used in another transactions inputs)
	// key is transaction ID where outputs belong
	// value is array of indexes for outputs
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for { // iterate through all blocks
		block := iter.Next()

		// iterate through all transactions on each block
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs { // iterate through all outputs of this transaction
				// outIdx is index of output in array

				// if the transaction contains some spent outputs (spent output = output that ends up being in another input therefore spent??)
				if spentTXOs[txID] != nil {

					// then iterate through all these spent outputs
					for _, spentOutIdx := range spentTXOs[txID] {
						// if the current output (outIdx) is a spent output in this transaction, skip to next output.
						// we only want to find outputs that have not been spent yet
						if spentOutIdx == outIdx {
							continue Outputs // skip to next output
						}
					}
				}
				// otherwise, reaching here means this transaction's output has not been spent yet
				// get all the unspent outputs for this transaction
				outs := UTXO[txID]
				// add the current output to the outputs array
				outs.Outputs = append(outs.Outputs, out)
				// set outputs array back to the unspent outputs map
				UTXO[txID] = outs
			}

			// iterate through all inputs for this transaction
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					// get the ID of the previous transaction that this input was in (when it was an output)
					inTxID := hex.EncodeToString(in.ID)
					// add the output index (when the input was previously an output) to the map
					// this marks the output as spent for that transaction
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 { // break if on genesis block
			break
		}
	}
	return UTXO
}

// OLD METHOD THAT DOESNT USE PERSISTENCE LAYER DB FOR UTXOs. NOT USED ANYMORE!
// Unspent transaction outputs for this particular pub key hash (adding up all of these will give the balance of wallet)
func (chain *BlockChain) FindUnspentTransactionOutputs(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput
	// gets all transactions where the outputs are unspent (not used as inputs in other transactions)
	unspentTransactions := chain.FindUnspentTransactions(pubKeyHash)

	for _, tx := range unspentTransactions { // iterate through all the outputs for all unspent transactions
		for _, out := range tx.Outputs {
			// check the output was locked with this address (belongs to this receiver and can be unlocked by this address to use as new input)
			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, out)
			}
		}
	}
	return UTXOs
}

// OLD METHOD THAT DOESNT USE PERSISTENCE LAYER DB FOR UTXOs. NOT USED ANYMORE!
// finds unspent outputs that you can use for the specified amount
func (chain *BlockChain) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	// contains unspent outputs for every transaction id (key)
	unspentOutputs := make(map[string][]int)
	// gets all transactions where the outputs are unspent (not used as inputs in other transactions)
	unspentTxs := chain.FindUnspentTransactions(pubKeyHash)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs { // iterate through unspent outputs

			// if output belongs to this public key hash and...
			// if accumulated is less than specified amount, keep adding unspent outputs together
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)

				// break out once accumulated outputs is equal to or larger than specified amount
				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOutputs
}

// iterates through all blocks until finding a transaction with ID
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return Transaction{}, errors.New("Transaction does not exist!")
}

// In order to sign the transaction, we need to take all the previous outputs that
// are being used as inputs for this transaction, and use the previous
// outputs pubkeyhash (created by hashing the receiver's address)
// to sign with our private key and create a digital signature which is attached to the new input as proof
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		// get all the previous transactions where the inputs were previously outputs
		// and add them to the map
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}
	// need to use the outputs in those previous transactions in order to sign the new
	// inputs created in this current transaction
	tx.Sign(privKey, prevTXs)
}

// In order to verify this current transaction, we need to check
// all the previous outputs used for this transactions inputs, and verify
// that these inputs were created from those outputs.
// this is done by using the previous outputs pubkeyhash and verifying it with
// the inputs public key + signature
// Note: anyone can verify a transaction, but only the sender of a transaction can sign it to prove the transaction is legitimate
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	// coinbase transaction has no inputs, so always return true
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}
	return tx.Verify(prevTXs)
}
