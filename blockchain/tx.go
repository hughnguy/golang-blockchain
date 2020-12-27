package blockchain

type TxOutput struct { // outputs are indivisible, cannot reference part of an output
	Value int // value in tokens assigned and locked into output
	PubKey string // receiver address: needed to unlock the tokens in value field (output can be used as input for another transaction)
}

type TxInput struct { // inputs are references to previous outputs
	ID []byte // references the transaction that the previous output was inside of
	Out int // an input was previously an output, therefore this Out variable is the index where the output previously appeared in the last transaction?
	Sig string // sender address. needed to verify this address used an output to create a new input
}

func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
