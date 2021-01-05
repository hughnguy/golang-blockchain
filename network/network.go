package network

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"golang-blockchain/blockchain"
	"gopkg.in/vrecan/death.v3"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"
)

/*
Algorithm:
- Create blockchain with central node
- Wallet node connects and downloads blockchain from central node
- Miner node connects and downloads blockchain from central node
- Wallet creates transaction
- Miner gets transaction and pushes to it's memory pool
- Once enough transactions are added to pool, miner mints block
- Minted block gets sent to central node
- Wallet node syncs and verifies that the node and payment was successful
*/

const (
	protocol = "tcp"
	version = 1
	commandLength = 12
)

// typically should use some kind of data/memory storage instead of variables like this
var (
	nodeAddress string // unique for each instance of a node (port we have open)
	minerAddress string // node address for node that is acting as miner
	KnownNodes = []string{"localhost:3000"} // main/central node is added by default
	blocksInTransit = [][]byte{}
	memoryPool = make(map[string]blockchain.Transaction) // key is the transaction ID
)

type Addr struct {
	AddrList []string // list of addresses connected to each node. allows us to discover nodes connected to other peers
}

type Block struct {
	AddrFrom string // address block is being built from
	Block []byte // the block itself
}

// allows us to get blockchain from one node and copy over to another node
type GetBlocks struct {
	AddrFrom string
}

type GetData struct {
	AddrFrom string
	Type string // transaction or block
	ID []byte
}

// inventory
type Inv struct {
	AddrFrom string
	Type string // transaction or block
	Items [][]byte
}

type Tx struct {
	AddrFrom string
	Transaction []byte
}

// allows us to sync blockchain between each of our nodes.
// when a server connects to one of our nodes, it will send it's version of the blockchain,
// and it will get a response of the node's version of the blockchain
type Version struct {
	// used to identify when a chain needs to be updated
	Version int
	// length of actual chain. if my blockchain is 4 blocks long and yours is 2, then you need to download the other 2 blocks and add to your chain
	BestHeight int
	AddrFrom string
}

// converts command into bytes
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}
	return bytes[:]
}

// deserialize bytes into command
func BytesToCmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		if b != 0x0 { // not zero. removes any spaces in command
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

func ExtractCmd(request []byte) []byte {
	return request[:commandLength]
}

// sends a list of known nodes + the current node connecting, to the node with address
func SendAddr(address string) {
	nodes := Addr{KnownNodes} // add existing known nodes
	nodes.AddrList = append(nodes.AddrList, nodeAddress) // include nodeAddress of this client that is connecting
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)

	// send command to address
	SendData(address, request)
}

// sends a block to the node with address
func SendBlock(addr string, b *blockchain.Block) {
	data := Block{nodeAddress, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)

	SendData(addr, request)
}

// sends inventory to the node with address
func SendInv(address, kind string, items [][]byte) {
	inventory := Inv{nodeAddress, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)

	SendData(address, request)
}

// sends transaction to the node with address
func SendTx(addr string, txn *blockchain.Transaction) {
	data := Tx{nodeAddress, txn.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(addr, request)
}

// sends current node's version to the node with address
func SendVersion(addr string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight() // get length of blockchain
	payload := GobEncode(Version{version, bestHeight, nodeAddress})
	request := append(CmdToBytes("version"), payload...)

	SendData(addr, request)
}

// sends request to node with address for their blocks on their blockchain
func SendGetBlocks(address string) {
	payload := GobEncode(GetBlocks{nodeAddress}) // this client wants to get blocks from node with address
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(address, request)
}

// sends data (id) to the node with address????
// kind can be block or transaction
func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(address, request)
}

func SendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr) // connect to internet using TCP

	// error means the node with addr is not connected, so remove that node from known nodes list
	if err != nil {
		fmt.Printf("%s is not available\n", addr)
		var updatedNodes []string

		// update the known nodes to remove the node with address
		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}

		KnownNodes = updatedNodes

		return
	}
	defer conn.Close()

	// call the connection and copy in data that is being sent to the other peer
	_, err = io.Copy(conn, bytes.NewReader(data))
	if err != nil {
		log.Panic(err)
	}
}

func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}

	command := BytesToCmd(req[:commandLength]) // convert bytes into command
	fmt.Printf("Received %s command\n", command)

	switch command {
	default:
		fmt.Println("Unknown command")
	}
}

// converts data into slice of bytes
// allows us to send commands to and from our application, including other structs
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// closes down the database if hitting control-c. intercepts that signal and closes down the DB and quitting app
func CloseDB(chain *blockchain.BlockChain) {
	// SIGINT and SIGTERM works for linux/osx. os.Interrupt works for windows
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit() // properly kills any goroutines running before exiting application
		chain.Database.Close() // gracefully close DB
	})
}
