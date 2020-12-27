package wallet

import (
	"github.com/mr-tron/base58"
	"log"
)
// Base58 was an algorithm invented with bitcoin
// Its a derivative of the base64 algorithms except it uses 6 less
// characters in its alphabet
// The characters missing are: 0 O l I + /
// The main reason they were taken out is because they can be easily confused
// with one another. We don't want people sending to incorrect addresses

func Base58Encode(input []byte) []byte {
	encode := base58.Encode(input)
	return []byte(encode)
}

func Base58Decode(input []byte) []byte {
	decode, err := base58.Decode(string(input[:]))
	if err != nil {
		log.Panic(err)
	}
	return decode
}
