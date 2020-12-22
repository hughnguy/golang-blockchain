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

type TxOutput struct { // outputs are indivisible, cannot reference part of an output
	Value int // value in tokens assigned and locked into output
	PubKey string // needed to unlock the tokens in value field
}

type TxInput struct { // inputs are references to previous outputs
	ID []byte // references the transaction that the output is inside of
	Out int // index where output appears
	Sig string
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

// generation transaction which has no parent and creates new coins from nothing
// coinbase can contain any arbitrary data
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}
	txin := TxInput{[]byte{}, -1, data} // references no output so empty byte array
	txout := TxOutput{100, to} // reward is 100

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}
	tx.SetID() // creates hash id for this transaction

	return &tx
}

func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	accumulated, validOutputs := chain.FindSpendableOutputs(from, amount)

	if accumulated < amount {
		log.Panic("Error: not enough funds")
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			input := TxInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	outputs = append(outputs, TxOutput{amount, to})

	if accumulated > amount {
		outputs = append(outputs, TxOutput{accumulated - amount, from})
	}

	tx := Transaction{nil, inputs, outputs}
	tx.SetID()

	return &tx
}

func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
