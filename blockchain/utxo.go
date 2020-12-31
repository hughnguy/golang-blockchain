package blockchain

import (
	"bytes"
	"encoding/hex"
	"github.com/dgraph-io/badger"
	"log"
)

var (
	utxoPrefix = []byte("utxo-") // prefix key used to namespace data in badger database
	prefixLength = len(utxoPrefix)
)

type UTXOSet struct {
	BlockChain *BlockChain
}

// finds unspent outputs that you can use for the specified amount
func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	// contains unspent outputs for every transaction id (key)
	unspentOutputs := make(map[string][]int)
	// gets all transactions where the outputs are unspent (not used as inputs in other transactions)
	accumulated := 0

	db := u.BlockChain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			Handle(err)
			// trim prefix and get transaction ID
			k = bytes.TrimPrefix(k, utxoPrefix)
			txID := hex.EncodeToString(k)
			outs := DeserializeOutputs(v)

			for outIdx, out := range outs.Outputs { // iterate through unspent outputs

				// if output belongs to this public key hash and...
				// if accumulated is less than specified amount, keep adding unspent outputs together
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
				}
			}
		}
		return nil
	})
	Handle(err)

	return accumulated, unspentOutputs
}

// Unspent transaction outputs for this particular pub key hash (adding up all of these will give the balance of wallet)
func (u UTXOSet) FindUnspentTransactions(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput

	db := u.BlockChain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		// iterate through all transactions with UTXOs
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			v, err := item.Value()
			Handle(err)
			outs := DeserializeOutputs(v)
			// go through all outputs of that transaction
			for _, out := range outs.Outputs {
				// check the output was locked with this address (belongs to this receiver and can be unlocked by this address to use as new input)
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}
		return nil
	})
	Handle(err)
	return UTXOs
}

// indexes the transaction IDs as a key with the value containing
// the serialized UTXOs of that transaction
func (u UTXOSet) Reindex() {
	db := u.BlockChain.Database

	u.DeleteByPrefix(utxoPrefix) // clears everything in database with this prefix

	UTXO := u.BlockChain.FindUTXO() // gets all UTXOs in the blockchain

	err := db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO { // iterate transaction IDS in UTXO map
			key, err := hex.DecodeString(txId) // get transaction ID as string
			if err != nil {
				return err
			}
			key = append(utxoPrefix, key...) // concatenate prefix to transaction ID: utxo-(transaction ID)

			// serialize and set these outputs as the value, with the key being utxo-(transaction ID)
			err = txn.Set(key, outs.Serialize())
			Handle(err)
		}
		return nil
	})
	Handle(err)
}

// updates the DB by removing any UTXO's from the database that are found as inputs in this block
func (u *UTXOSet) Update(block *Block) {
	db := u.BlockChain.Database

	err := db.Update(func(txn *badger.Txn) error {
		for _, tx := range block.Transactions { // iterate through all transactions in this block
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs { // iterate all inputs in this transaction
					// contains updated UTXO's which have not been used as inputs
					updatedOuts := TxOutputs{}
					// create key for transaction ID that this input's previous output was inside of: utxo-(transaction-id)
					inID := append(utxoPrefix, in.ID...)
					// this transaction contains an array of UTXOs (outputs which have not been spent yet)
					item, err := txn.Get(inID)
					Handle(err)
					v, err := item.Value()
					Handle(err)

					// converts bytes back into array of outputs
					outs := DeserializeOutputs(v)

					for outIdx, out := range outs.Outputs {
						// we want to remove any new input's previous output from the list of UTXO's
						// stored in this transaction, since that output has now become an input and is not a UTXO anymore.
						// only add outputs that do not reference this input to the new empty UTXO array
						if outIdx != in.Out {
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					// delete the key if this transaction no longer has any UTXOs
					if len(updatedOuts.Outputs) == 0 {
						if err := txn.Delete(inID); err != nil {
							log.Panic(err)
						}
					} else {
						// otherwise, serialize the new updated UTXO list and store in the transaction ID key
						if err := txn.Set(inID, updatedOuts.Serialize()); err != nil {
							log.Panic(err)
						}
					}
				}
			}
			// all outputs in this transaction end up becoming new UTXOs so we add them to the array and store them in DB. this logic below also handles coinbase transactions with no inputs.
			// the next transaction iteration will check inputs and could potentially remove some of these UTXOs added here
			// this works because the Update() function is being called in order, sequentially, from the very first block up until the very last??
			// calling this on 1 random block only could mess up the chain since it overrides the UTXOs??? you'd have to keep calling Update() up until the very last block??
			newOutputs := TxOutputs{}
			for _, out := range tx.Outputs {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}
			txID := append(utxoPrefix, tx.ID...)
			if err := txn.Set(txID, newOutputs.Serialize()); err != nil {
				log.Panic(err)
			}
		}
		return nil
	})
	Handle(err)
}

// counts how many transactions contain UTXOs in the DB
func (u UTXOSet) CountTransactions() int {
	db := u.BlockChain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		// iterate through all keys with utxo prefix
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}
		return nil
	})
	Handle(err)
	return counter
}

// deletes in bulk the prefix keys in DB
func (u *UTXOSet) DeleteByPrefix(prefix []byte) {

	// create function that deletes keys
	deleteKeys := func(keysForDelete [][]byte) error {
		if err := u.BlockChain.Database.Update(func(txn *badger.Txn) error {
			// iterate through keys and delete them (each key is an array of bytes)
			for _, key := range keysForDelete {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	collectSize := 100000 // batch size to delete per iteration
	// read only operation
	u.BlockChain.Database.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		// makes it so we can read the keys but dont need to fetch the values
		// since we dont need to read the values when deleting
		opts.PrefetchValues = false
		it := txn.NewIterator(opts) // create iterator with the options above
		defer it.Close()

		// create array of bytes with batch size
		keysForDelete := make([][]byte, 0, collectSize)
		keysCollected := 0
		// seek all keys with prefix. ValidForPrefix() returns false when done iterating prefixes
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().KeyCopy(nil) // copy key
			keysForDelete = append(keysForDelete, key) // add to array
			keysCollected++
			// once reaching batch size, delete keys in array and reset counter + array
			if keysCollected == collectSize {
				if err := deleteKeys(keysForDelete); err != nil {
					log.Panic(err)
				}
				keysForDelete = make([][]byte, 0, collectSize)
				keysCollected = 0
			}
		}
		// delete any remaining keys in last batch
		if keysCollected > 0 {
			if err := deleteKeys(keysForDelete); err != nil {
				log.Panic(err)
			}
		}
		return nil
	})
}
