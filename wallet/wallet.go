package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"golang.org/x/crypto/ripemd160"
)
/*
- Step 1: use ecdsa to create private key and public key
- Step 2: use sha256 on public key
- Step 3: run result of that on ripemd160 hashing algorithm which creates "public key hash"
- Step 4: take the "public key hash" and run through sha256 algorithm twice
- Step 5: take off the first 4 bytes of the resulting hash, this becomes the checksum

- Step 6: also take the original "public key hash" + "version" + "checksum" and pass it through base58 encoder algorithm
- the result is the "address"
- the "version" indicates where in the blockchain this address resides
 */

const (
	checksumLength = 4 // in bytes
	version = byte(0x00) // hexadecimal representation of zero
)

type Wallet struct {
	PrivateKey ecdsa.PrivateKey // elliptical curve digital signing algorithm (algorithm used to generate private and public key)
	PublicKey []byte
}

// Bitcoin address is a hash of the public key
// A hash is generated from a one way hash function which produces the same length result
// sha256 result is 256 bits long
func (w Wallet) Address() []byte {
	// Step 2 and 3
	pubKeyHash := PublicKeyHash(w.PublicKey)

	// all addresses start with 1 because of the version appended to hash
	versionedHash := append([]byte{version}, pubKeyHash...)
	// Step 4 and 5
	checksum := Checksum(versionedHash)

	// Step 6: take the original "public key hash" + "version" + "checksum"
	// and pass it through base58 encoder algorithm
	fullHash := append(versionedHash, checksum...)
	address := Base58Encode(fullHash)

	fmt.Printf("public key: %x\n", w.PublicKey)
	fmt.Printf("public key hash: %x\n", pubKeyHash)
	fmt.Printf("public address: %s\n", address)

	return address
}

/*
- Validation is done by getting the actual checksum appended to the address,
taking the data portion (version + public key hash), and running that through the
original checksum generation function. You can then compare that generated target
checksum with the actual checksum to validate the address is correct

- Takes in bitcoin address
- Convert into full hash by passing bitcoin address into base58 decoder
- Get the actual checksum appended at the end of full hash
- Rip out the version portion which is the first byte (or 2 characters)
- Take off the remaining public key hash portion
- Take public key hash, attach version constant to it, and pass it back through checksum function to create new checksum
- Compare actual check sum with the target check sum
 */
func ValidateAddress(address string) bool {
	// Convert into full hash by passing bitcoin address into base58 decoder
	pubKeyHash := Base58Decode([]byte(address))
	// Get the actual checksum appended at the end of full hash
	actualCheckSum := pubKeyHash[len(pubKeyHash) - checksumLength:]
	// Rip out the version portion which is the first byte (or 2 characters)
	version := pubKeyHash[0]
	// Take off the remaining public key hash portion
	pubKeyHash = pubKeyHash[1:len(pubKeyHash)-checksumLength]
	targetCheckSum := Checksum(append([]byte{version}, pubKeyHash...))

	return bytes.Compare(actualCheckSum, targetCheckSum) == 0
}

// Step 1: use ecdsa to create private key and public key
func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()

	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}

	public := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...) // unpacks byte array and passes in each byte as an additional argument to the variadic function
	return *private, public
}

func MakeWallet() *Wallet {
	// Step 1: use ecdsa to create private key and public key
	private, public := NewKeyPair()
	wallet := Wallet{private, public}
	return &wallet
}

func PublicKeyHash(pubKey []byte) []byte {
	// Step 2: use sha256 on public key
	pubHash := sha256.Sum256(pubKey)

	// Step 3: run result of that on ripemd160 hashing algorithm which creates "public key hash"
	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	if err != nil {
		log.Panic(err)
	}

	publicRipMD := hasher.Sum(nil)
	return publicRipMD
}

// checksum/hash is used for data integrity. Ensures no errors in files during transmission or storage
// ex: If you know the checksum of the original file, you can run a checksum or hashing utility on it.
// If the resulting checksum matches, you know the file you have is identical.
func Checksum(payload []byte) []byte {
	// Step 4: take the "public key hash" and run through sha256 algorithm twice
	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	// Step 5: take off the first 4 bytes of the resulting hash, this becomes the checksum
	return secondHash[:checksumLength]
}
