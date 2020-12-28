package blockchain

import (
	"bytes"
	"golang-blockchain/wallet"
)

type TxOutput struct { // outputs are indivisible, cannot reference part of an output
	Value int // value in tokens assigned and locked into output
	// This is a byte representation of your address (address is a hash of public key with checksum + version constant added to it)
	// but with the checksum and version constant removed.
	// generated when new output is created and locked with receiver's address.
	// this pubkeyhash is later used when creating a signature for a new input. it has this hash attached to
	// the input before hashing the transaction and signing with your private key.
	// the signature proves that the new input was previously created by this output (which is owned/locked by you)
	PubKeyHash []byte
}

type TxInput struct { // inputs are references to previous outputs
	ID []byte // references the transaction that the previous output was inside of
	Out int // an input was previously an output, therefore this Out variable is the index where the output previously appeared in the last transaction
	// signature created by using previous output's pubkeyhash and
	Signature []byte
	// the pubkey field is also used to help generate the signature
	// this field is used to match up with a referenced output public key hash
	// we temporarily attach the previous output's pubkeyhash field here before hashing and signing the entire transaction with a private key.
	// the signature generated proves that this input was created by the same person who owned the previous output
	PubKey []byte // normally this field would just contain the public key of the sender address
}

// locks the output by getting the public key hash from the address.
// this hash will later be used when signing a transaction with your private key to create new inputs
// this signature added to the input proves that you (not someone else) created a new input using this output
func NewTxOutput(value int, address string) *TxOutput {
	txo := &TxOutput{value, nil}
	txo.Lock([]byte(address))
	return txo
}

func (in *TxInput) UsesKey(pubKeyHash []byte) bool {
	lockingHash := wallet.PublicKeyHash(in.PubKey)

	return bytes.Compare(lockingHash, pubKeyHash) == 0
}

// gets public key hash from address to lock the output
// note: remember the address is a hashed public key prepended with version and appended checksum
// that's all base58 encoded
func (out *TxOutput) Lock(address []byte) {
	// base58 decode the address
	pubKeyHash := wallet.Base58Decode(address)
	pubKeyHash = pubKeyHash[1:len(pubKeyHash)-4] // remove version byte and checksum from hash
	// this gives us the public key hash
	out.PubKeyHash = pubKeyHash
}

func (out *TxOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Compare(out.PubKeyHash, pubKeyHash) == 0
}

//func (in *TxInput) CanUnlock(data string) bool {
//	return in.Sig == data
//}
//
//func (out *TxOutput) CanBeUnlocked(data string) bool {
//	return out.PubKey == data
//}
