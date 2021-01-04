package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dgraph-io/badger"
	"os"
	"runtime"
)

const (
	dbPath = "./tmp/blocks"
	dbFile = "./tmp/blocks/MANIFEST" // used to verify whether or not blockchain database exists
	genesisData = "First Transaction from Genesis"
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

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

func InitBlockChain(address string) *BlockChain { // creates blockchain
	var lastHash []byte

	if DBexists() {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
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

func ContinueBlockChain(address string) *BlockChain {
	if DBexists() == false {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
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

func (chain *BlockChain) AddBlock(transactions []*Transaction) *Block {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(lastHashKey))
		Handle(err)
		lastHash, err = item.Value()
		return err
	})
	Handle(err)

	newBlock := CreateBlock(transactions, lastHash)

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
