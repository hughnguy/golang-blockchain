package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

const walletFile = "./tmp/wallets_%s.data"

type Wallets struct {
	Wallets map[string]*Wallet // address is key
}

// loads wallets that already exist from this node
func CreateWallets(nodeId string) (*Wallets, error) {
	wallets := Wallets{}
	wallets.Wallets = make(map[string]*Wallet) // creates map to store wallets

	err := wallets.LoadFile(nodeId) // loads wallets from file
	return &wallets, err
}

func (ws *Wallets) AddWallet() string {
	wallet := MakeWallet()
	address := fmt.Sprintf("%s", wallet.Address()) // get bytes and convert into string

	ws.Wallets[address] = wallet

	return address
}

func (ws *Wallets) GetAllAddresses() []string {
	var addresses []string

	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}
	return addresses
}

func (ws Wallets) GetWallet(address string) (*Wallet, error) {
	if wallet, exists := ws.Wallets[address]; exists {
		return wallet, nil
	} else {
		return nil, errors.New("This node does not own the private key for wallet: " + address)
	}
}

func (ws *Wallets) LoadFile(nodeId string) error {
	walletFile := fmt.Sprintf(walletFile, nodeId)

	if _, err := os.Stat(walletFile); os.IsNotExist(err) {
		return err
	}
	var wallets Wallets

	fileContent, err := ioutil.ReadFile(walletFile)
	if err != nil {
		return err
	}
	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileContent))
	err = decoder.Decode(&wallets)
	if err != nil {
		return err
	}

	ws.Wallets = wallets.Wallets

	return nil
}

func (ws *Wallets) SaveFile(nodeId string) {
	var content bytes.Buffer
	walletFile := fmt.Sprintf(walletFile, nodeId)

	gob.Register(elliptic.P256())

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)

	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(walletFile, content.Bytes(), 0644) // give read and write permissions
	if err != nil {
		log.Panic(err)
	}
}
