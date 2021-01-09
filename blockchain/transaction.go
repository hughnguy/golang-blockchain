package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"golang-blockchain/wallet"
	"log"
	"math/big"
	"strings"
)

type Transaction struct  {
	ID []byte
	Inputs []TxInput
	Outputs []TxOutput
}

// serializes transaction into bytes
func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

// deserializes bytes into transaction
func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&transaction)
	Handle(err)
	return transaction
}

// creates a hash of the transaction which we can use as a transaction ID
func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())
	return hash[:]
}

// generation transaction which has no parent inputs and creates new coins from nothing (only has outputs)
// coinbase can contain any arbitrary data
func CoinbaseTx(to, data string) *Transaction {
	// creates random generated data
	if data == "" {
		randData := make([]byte, 24)
		_, err := rand.Read(randData)
		if err != nil {
			log.Panic(err)
		}
		data = fmt.Sprintf("%x", randData)
	}
	txin := TxInput{[]byte{}, -1, nil, []byte(data)} // references no output so empty byte array and negative index for Out variable
	txout := NewTxOutput(20, to) // reward is 20 tokens

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}}
	tx.ID = tx.Hash() // creates hash id for this transaction

	return &tx
}

func NewTransaction(w *wallet.Wallet, to string, amount int, UTXO *UTXOSet) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	// get hash of public key (this is different from address. address also contains version + checksum)
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	// gets all outputs that are unspent, which can be spent for the specified amount
	// accumulated amount can be larger than the specified amount since adding UTXOs which are not divisible
	accumulated, validOutputs := UTXO.FindSpendableOutputs(pubKeyHash, amount)
	// old inefficient method (does not use DB)
	// accumulated, validOutputs := chain.FindSpendableOutputs(pubKeyHash, amount)

	if accumulated < amount {
		log.Panic("Error: not enough funds")
	}

	// go through all unspent outputs which can be used as new inputs
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			// the txID here references the previous transaction that the output was inside of
			input := TxInput{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	// get wallet address
	from := fmt.Sprintf("%s", w.Address())

	// output here is created for the amount to send. we lock the output with the receiver address
	// so that only they can use this output for future inputs
	outputs = append(outputs, *NewTxOutput(amount, to))

	// if accumulated is more than specified amount, create a new output to send back to original wallet
	if accumulated > amount {
		outputs = append(outputs, *NewTxOutput(accumulated - amount, from))
	}

	// add inputs and outputs to the transaction
	tx := Transaction{nil, inputs, outputs}
	// create ID for transaction by hashing it
	tx.ID = tx.Hash()
	// signing transaction with private key proves that the new inputs are created by
	// previous outputs that are owned by you (only you can unlock)
	UTXO.BlockChain.SignTransaction(&tx, w.PrivateKey)

	return &tx
}

func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// Transaction is signed using your private key. This is a digital signature
// which proves that you created the transaction and not someone else
// note: the key in map is hash of transaction (transaction ID)
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTxs map[string]Transaction) {
	if tx.IsCoinbase() {
		return // do not need to sign the coinbase transaction
	}

	// check that the transactions that these inputs were previously in (when they were outputs) exist
	for _, in := range tx.Inputs {
		// in.ID is the transaction that the input was previously in, when it was an output
		if prevTxs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction does not exist")
		}
	}

	// clears the signature and public key in the inputs of this transaction
	// before hashing the transaction for the iterated input and signing that input
	txCopy := tx.TrimmedCopy() // creates new copy, not a reference to old one

	for inId, in := range txCopy.Inputs {
		// get previous transaction this input was inside of when it was an output
		prevTx := prevTxs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil // this should have already been done in the TrimmedCopy()
		// get the public key hash of the input (when it was previously an output)
		// and assign this to the public key of the new input being created, before hashing the transaction
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash
		// hash the transaction to create a generated hash (ID) which will be later signed. when hashing the transaction,
		// all other inputs should have a nil pubkey except for this current input being iterated
		txCopy.ID = txCopy.Hash()
		// clean up the pubkey for the current input so that data is not corrupt
		// when hashing the transaction on next input iteration
		txCopy.Inputs[inId].PubKey = nil

		// signs the generated hash (ID), using a private key to generate a signature.
		// this signature proves that the new inputs being created are made by you
		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		Handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		// attach signature to this input as proof
		tx.Inputs[inId].Signature = signature
	}
}

// When verifying a transaction, we are taking the public key in the inputs, and checking
// that they match with a referenced output that contains a hash of the public key inside of it
func (tx *Transaction) Verify(prevTxs map[string]Transaction) bool {
	if tx.IsCoinbase() {
		return true // coinbase transaction is never signed
	}

	for _, in := range tx.Inputs {
		if prevTxs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("Previous transaction does not exist")
		}
	}

	// clears the signature and public key in the inputs of this transaction
	// before hashing the transaction for the iterated input
	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	for inId, in := range tx.Inputs {
		prevTx := prevTxs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil // this should have already been done in the TrimmedCopy()
		// get the public key hash of the input (when it was previously an output)
		// and assign this to the public key of the input being verified, before hashing the transaction
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash
		// hash the transaction to create a generated hash (ID) which will later be verified. when hashing the transaction,
		// all other inputs should have a nil pubkey except for this current input being iterated
		txCopy.ID = txCopy.Hash()
		// clean up the pubkey for the current input so that data is not corrupt
		// when hashing the transaction on next input iteration
		txCopy.Inputs[inId].PubKey = nil

		// need to unpack all the data we put into the signature and pubkey of transaction input (previous output's public key hash)
		r := big.Int{}
		s := big.Int{}
		sigLen := len(in.Signature)
		// unpack the signature used for this input
		r.SetBytes(in.Signature[:(sigLen/2)])
		s.SetBytes(in.Signature[:(sigLen/2)])

		x := big.Int{}
		y := big.Int{}
		keyLen := len(in.PubKey)
		// unpack the public key (sender) used for this input
		x.SetBytes(in.PubKey[:(keyLen/2)])
		y.SetBytes(in.PubKey[(keyLen/2):])

		rawPubKey := ecdsa.PublicKey{curve, &x, &y}

		// after unpacking all the data from the signature of this current input and
		// the public key hash of the previous output (which was locked with an address and
		// got attached to this current input before hashing the entire transaction),
		// running verify() here proves that this input was created and signed from the same person
		// who owned the previous output (before being used in this input)
		if ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) == false {
			return false
		}
	}
	// all inputs are verified therefore transaction is verified
	return true
}


// clears the signature and pubkey fields for all inputs in this transaction
func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	for _, in := range tx.Inputs {
		// clear signature and public key
		inputs = append(inputs, TxInput{in.ID, in.Out, nil, nil})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TxOutput{out.Value, out.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs}

	return txCopy
}

// printing the transaction calls this String() method
func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))
	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("     Input %d:", i))
		lines = append(lines, fmt.Sprintf("       TXID:     %x", input.ID))
		lines = append(lines, fmt.Sprintf("       Out:       %d", input.Out))
		lines = append(lines, fmt.Sprintf("       Signature: %x", input.Signature))
		lines = append(lines, fmt.Sprintf("       PubKey:    %x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("     Output %d:", i))
		lines = append(lines, fmt.Sprintf("       Value:  %d", output.Value))
		lines = append(lines, fmt.Sprintf("       Script: %x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}

