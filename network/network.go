package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
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
	minPoolSizeBeforeMining = 2 // number of tx in pool before mining a block
)

// typically should use some kind of data/memory storage instead of variables like this
var (
	nodeAddress string // unique for each instance of a node (port we have open)
	minerAddress string // node address for node that is acting as miner
	KnownNodes = []string{"localhost:3000"} // main/central node is added by default
	blocksInTransit = [][]byte{}
	// pool of transactions
	memoryPool = make(map[string]blockchain.Transaction) // key is the transaction ID
)

type Addr struct {
	AddrList []string // list of addresses for each node. allows us to discover nodes connected to other peers
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

// iterates through all known nodes of this client, and requests each of the
// nodes to send the blocks on their blockchain
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
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

// prompts the node at address that they need to update their inventory with this block or transaction.
// the other nodes will then set the inventory as pending delivery, and request the full data of that block or transaction to be sent back
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

// sends request to node with address and asks for the blocks on their blockchain
func SendGetBlocks(address string) {
	payload := GobEncode(GetBlocks{nodeAddress}) // this client wants to get blocks from node with address
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(address, request)
}

// sends request to node with address and asks to get the particular data (id). either block or transaction
// kind can be block or transaction
func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(address, request)
}

// sends data in bytes to a specific node (address)
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

// handles receiving the "addr" command from another peer,
// where another peer is sending this client all of it's known nodes (AddrList)
// takes in "addr" command as a bunch of bytes
func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	// decode the request into an Addr struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	// add the known nodes to this current node's known nodes
	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))

	// after updating this client's list of known nodes,
	// we want to request all of the nodes for the blocks on their blockchain
	RequestBlocks()
}

// handles receiving the "inv" command from another peer,
// where another peer is prompting this client that it need to update it's inventory with a new block or transaction.
// this client will then request back the full data it needs (block or transaction)
func HandleInv(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inv

	// decode the request into an Inv struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	// prints the number of inventory type (block or transaction), which should be updated
	fmt.Printf("Received inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		// set blocks in transit (pending delivery to this client, so that we can update our inventory)
		blocksInTransit = payload.Items

		// get the block hash we need and send a request back in order to get that block
		blockHash := payload.Items[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		// remove the blockhash (of the block requested to be sent back) from transit
		newInTransit := [][]byte{}
		for _, b := range blocksInTransit {
			if bytes.Compare(b, blockHash) != 0 {
				newInTransit = append(newInTransit, b)
			}
		}
		// update blocks in transit with the blockhash above removed.
		// its not in transit anymore since the full block has been requested to be sent back
		blocksInTransit = newInTransit
	}

	// if the transaction ID does not exist in this client's pool, then request for the full transaction to be sent back
	if payload.Type == "tx" {
		txID := payload.Items[0]

		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(payload.AddrFrom, "tx", txID)
		} else {
			fmt.Println("Pool already contains tx. Do not add to pool.")
		}
	}
}

// handles receiving the "block" command from another peer,
// where another peer sends this client a block that has just been mined?
func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	// decode the request into a Block struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	// convert block bytes into blockchain package's Block Struct
	blockData := payload.Block
	block := blockchain.Deserialize(blockData)

	// add the sent block to this clients blockchain
	fmt.Printf("Received a new block! ")
	chain.AddBlock(block)

	fmt.Printf("Added block %x\n", block.Hash)

	// if there are any blocks currently in transit
	if len(blocksInTransit) > 0  {
		// take first block in transit and assign to blockHash
		blockHash := blocksInTransit[0]
		// ask the sender of the block that we received, for the data of this block in transit
		SendGetData(payload.AddrFrom, "block", blockHash)

		// remove the first block from blocksInTransit
		blocksInTransit = blocksInTransit[1:]
	} else {
		// if no more blocks in transit, just reindex the UTXO set since a new block has been added to the chain
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()
	}
}

// handles receiving the "getblocks" command from another peer,
// where another peer is requesting for this client to send all its blocks on this blockchain
func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	// decode the request into a GetBlocks struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)

	if err != nil {
		log.Panic(err)
	}

	// get all the hashes from this blockchain and send the inventory to the peer who requested it.
	// this tells the other node they must update their inventory with these block hashes.
	// the other node will then request back the full data
	blocks := chain.GetBlockHashes()
	SendInv(payload.AddrFrom, "block", blocks)
}

// handles receiving the "getdata" command from another peer,
// when another peer is requesting for this client to send them either a specific block or transaction
func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	// decode the request into a GetData struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// if the sender is requesting a block from this client, then get the block and send it back to them
	if payload.Type == "block" {
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}
		SendBlock(payload.AddrFrom, &block)
	}

	// if the sender is requesting a transaction, get it from your memory pool and send it back to them
	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTx(payload.AddrFrom, &tx)
	}
}

// adds all verified transactions from the memory pool into a block
// and attempts to mine that block
func MineTx(chain *blockchain.BlockChain) {
	var txs []*blockchain.Transaction

	// iterate through all transactions in the memory pool
	for id := range memoryPool {
		fmt.Printf("tx: %s\n", memoryPool[id].ID)
		tx := memoryPool[id]
		// if transaction is verified, then add it to list
		if chain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		}
	}

	// if no transactions in the pool are verified (inputs come from existing outputs)
	if len(txs) == 0 {
		fmt.Println("All transactions are invalid")
		return
	}

	// create coinbase transaction which rewards the miner. no inputs and only has output to miner address
	cbTx := blockchain.CoinbaseTx(minerAddress, "")
	txs = append(txs, cbTx)

	// mine the block and reindex the UTXOSet on completion
	newBlock := chain.MineBlock(txs)

	// the above mining should take some time^^
	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Println("New Block mined")

	// delete all the transactions added to the block from this node's memory pool
	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	// tell all other nodes that this block has just been mined.
	// tells other nodes they need to update their inventory with this block hash. the other nodes will then request for the full data (block)
	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	// recursion: keep mining if new transactions were added to the memory pool while the previous block was being mined
	if len(memoryPool) > 0 {
		MineTx(chain)
	}
}

// handles receiving the "version" command from another peer,
// when another peer is sending its blockchain version to this client
func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	// decode the request into a Version struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// get length of this current chain and compare with other chain
	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	// if other chain is longer, request the blocks from that node
	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight {
		// otherwise if this chain is longer, send its version to the other node.
		// the other node will then send a request to get this chains block
		SendVersion(payload.AddrFrom, chain)
	}

	// add node to list of known nodes, if this node is receiving a message from it for the first time
	if !NodeIsKnown(payload.AddrFrom) {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

// handles receiving the "tx" command from another peer,
// when another peer is sending a transaction to this client, which we add to our memory pool
func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	// decode the request into a Tx struct (payload)
	// trim the command off the beginning of the bytes
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// deserialize transaction sent to this client
	txData := payload.Transaction
	tx := blockchain.DeserializeTransaction(txData)
	// add that transaction to this nodes memory pool
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	// print this node's address and length of memory pool
	fmt.Printf("%s now contains %d transactions in it's pool\n", nodeAddress, len(memoryPool))

	// if this node is the main/central node (first one added to list by default: port 3000)
	if nodeAddress == KnownNodes[0] {
		for _, node := range KnownNodes {
			// send inventory (transaction ID) to all other nodes except for this one and the node which sent this transaction.
			// this tells other nodes they must update their inventory with this transaction ID. the other nodes will then request back this transaction
			if node != nodeAddress && node != payload.AddrFrom {
				SendInv(node, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		// otherwise if this is a miner node (miner address longer than 0), and we have more than a specified # transactions in the memory pool, mine new block with these transactions
		if len(memoryPool) > minPoolSizeBeforeMining && len(minerAddress) > 0 {
			MineTx(chain)
		}
		// if not a main/central node or a miner node then the client node simply listens and updates its blockchain
	}
}

func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	// read connection to see if any data has been sent to this node (address)
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}

	command := BytesToCmd(req[:commandLength]) // convert bytes into command
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, chain)
	case "inv":
		HandleInv(req, chain)
	case "getblocks":
		HandleGetBlocks(req, chain)
	case "getdata":
		HandleGetData(req, chain)
	case "tx":
		HandleTx(req, chain)
	case "version":
		HandleVersion(req, chain)
	default:
		fmt.Println("Unknown command")
	}
}

func StartServer(nodeID, mineAddress string) { // mineAddress is optional
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)

	// optionally make this node a miner (which will send reward to this address)
	minerAddress = mineAddress

	// start server at port (nodeID)?
	// A Listener is a generic network listener for stream-oriented protocols.
	ln, err := net.Listen(protocol, nodeAddress)
	if err != nil {
		log.Panic(err)
	}
	defer ln.Close()

	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()
	go CloseDB(chain) // goroutine which will listen and close database

	// if this client is not the main/central node, then send this client's blockchain version to the central node
	if nodeAddress != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	// infinite loop which listens to the connection and handles messages sent to this node
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Panic(err)
		}
		go HandleConnection(conn, chain) // make it a go routine so it can be async
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

// checks if address belongs to list of known nodes
func NodeIsKnown(addr string) bool {
	for _, node := range KnownNodes {
		if node == addr {
			return true
		}
	}
	return false
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
