package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

type Transaction struct  {
	ID []byte
	Inputs []TxInput
	Outputs []TxOutput
}

// create hash ID from transaction
func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encode := gob.NewEncoder(&encoded)
	err := encode.Encode(tx)
	Handle(err)

	hash = sha256.Sum256(encoded.Bytes()) // hash the encoded transaction
	tx.ID = hash[:]
}

// generation transaction which has no parent inputs and creates new coins from nothing (only has outputs)
// coinbase can contain any arbitrary data
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}
	txin := TxInput{[]byte{}, -1, data} // references no output so empty byte array and negative index for Out variable
	txout := TxOutput{100, to} // reward is 100

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}
	tx.SetID() // creates hash id for this transaction

	return &tx
}

func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	// gets all outputs that are unspent, which can be spent for the specified amount
	// accumulated amount can be larger than the specified amount since adding UTXOs which are not divisible
	accumulated, validOutputs := chain.FindSpendableOutputs(from, amount)

	if accumulated < amount {
		log.Panic("Error: not enough funds")
	}

	// go through all unspent outputs which can be used
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			// the txID here references the previous transaction that the output was inside of
			input := TxInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	// output here is created for the amount to send
	outputs = append(outputs, TxOutput{amount, to})

	// if accumulated is more than specified amount, create a new output to send back to original wallet
	if accumulated > amount {
		outputs = append(outputs, TxOutput{accumulated - amount, from})
	}

	// add inputs and outputs to the transaction
	tx := Transaction{nil, inputs, outputs}
	tx.SetID()

	return &tx
}

func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}
