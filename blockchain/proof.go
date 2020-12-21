package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/big"
)

// 1) Take the data from the block

// 2) Create a counter (nonce) which starts at 0

// 3) Create a hash of the data + the counter

// 4) Check the hash to see if it meets a set of requirements

// Requirements:
// The first few bytes must contain 0s

const Difficulty = 12

type ProofOfWork struct {
	Block *Block
	Target *big.Int
}

func NewProof(b *Block) *ProofOfWork {
	const numberOfBytesInHash = 256

	target := big.NewInt(1)
	target.Lsh(target, uint(numberOfBytesInHash - Difficulty)) // left shift bytes

	pow := &ProofOfWork{b, target} // creates proof of work for block

	return pow
}

func (pow *ProofOfWork) InitData(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.Block.PrevHash,
			pow.Block.Data,
			ToHex(int64(nonce)), // converts nonce to hex
			ToHex(int64(Difficulty)), // convert difficulty to hex
		},
		[]byte{},
	)
	return data
}

func (pow *ProofOfWork) Run() (int, []byte) {
	var intHash big.Int
	var hash [32]byte

	nonce := 0

	for nonce < math.MaxInt64 {
		// the correct nonce is the number found, which combined with the data,
		// will create a hash containing x number of 0's at the beginning (depending on the difficulty)

		data := pow.InitData(nonce) // Step 1,2,3 occurs here (create hash of block data and nonce)
		hash := sha256.Sum256(data) // Step 3: hash is created here

		fmt.Printf("\r%x", hash)
		intHash.SetBytes(hash[:]) // convert hash into big int

		if intHash.Cmp(pow.Target) == -1 { // compare hash with pow target
			break // if intHash is smaller than pow target, the work has been solved
		} else {
			nonce++
		}
	}
	fmt.Println()

	return nonce, hash[:]
}

func (pow *ProofOfWork) Validate() bool {
	var intHash big.Int

	data := pow.InitData(pow.Block.Nonce)

	hash := sha256.Sum256(data) // convert data into hash
	intHash.SetBytes(hash[:]) // convert hash into big int

	return intHash.Cmp(pow.Target) == -1 // check if valid (valid when intHash is smaller than pow target)
}

func ToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}
