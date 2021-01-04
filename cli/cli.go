package cli

import (
	"flag"
	"fmt"
	"golang-blockchain/blockchain"
	"golang-blockchain/wallet"
	"log"
	"os"
	"runtime"
	"strconv"
)

type CommandLine struct {}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage:")

	fmt.Println(" getbalance -address ADDRESS - Get the balance for an address")
	fmt.Println(" createblockchain -address ADDRESS - Creates a blockchain and sends reward to address")
	fmt.Println(" printchain - Prints the blocks in the chain")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT - Send amount of coins")

	fmt.Println(" createwallet - Creates a new wallet")
	fmt.Println(" listaddresses - Lists the addresses in our wallet file")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
}

// validates arguments passed to command line
func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit() // allows badger DB to properly shutdown without corrupting data
	}
}

func (cli *CommandLine) reindexUTXO() {
	chain := blockchain.ContinueBlockChain("")
	defer chain.Database.Close()
	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) listAddresses() {
	wallets, _ := wallet.CreateWallets()
	addresses := wallets.GetAllAddresses()

	for _ ,address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet() {
	wallets, _ := wallet.CreateWallets()
	address := wallets.AddWallet()
	wallets.SaveFile()

	fmt.Printf("New address is: %s\n", address)
}

func (cli *CommandLine) printChain() {
	chain := blockchain.ContinueBlockChain("")
	defer chain.Database.Close()
	iter := chain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)
		pow := blockchain.NewProof(block) // validates block
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		for _, tx := range block.Transactions {
			// printing the transaction calls the Transaction.String() method
			fmt.Println(tx)
		}
		fmt.Println()

		if len(block.PrevHash) == 0 { // genesis block has empty byte array as previous hash
			break
		}
	}
}

func (cli *CommandLine) createBlockChain(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not valid")
	}
	chain := blockchain.InitBlockChain(address)
	defer chain.Database.Close()

	// update UTXO set so that reward for genesis block appears in database
	// can also run UTXO.update() instead inside of blockchain.InitBlockChain()???
	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Println("Finished!")
}

// Balance is calculated by adding up all UTXOs (unspent transaction outputs)
func (cli *CommandLine) getBalance(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not valid")
	}

	chain := blockchain.ContinueBlockChain(address)
	UTXOSet := blockchain.UTXOSet{chain}
	defer chain.Database.Close()

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1:len(pubKeyHash) - 4] // need to remove version and checksum from address to get public key hash
	UTXOs := UTXOSet.FindUnspentTransactionOutputs(pubKeyHash)

	// old inefficient method (does not use DB)
	//UTXOs := chain.FindUTXOForPubKeyHash(pubKeyHash)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) send(from, to string, amount int) {
	if !wallet.ValidateAddress(to) {
		log.Panic("Address is not valid")
	}
	if !wallet.ValidateAddress(from) {
		log.Panic("Address is not valid")
	}
	chain := blockchain.ContinueBlockChain(from)
	UTXOSet := blockchain.UTXOSet{chain}
	defer chain.Database.Close()

	tx := blockchain.NewTransaction(from, to, amount, &UTXOSet)

	// create the coinbase transaction here as well since this sender is also mining the block
	cbTx := blockchain.CoinbaseTx(from, "")

	block := chain.AddBlock([]*blockchain.Transaction{cbTx, tx}) // add coinbase transaction to block as well

	// updates the UTXO database after creating new block
	UTXOSet.Update(block)

	fmt.Println("Success!")
}

func (cli *CommandLine) Run() {
	cli.validateArgs()

	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	createWalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listAddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)
	reindexUTXOCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)

	getBalanceAddress := getBalanceCmd.String("address", "", "The address")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")

	switch os.Args[1] {
	case "reindexutxo":
		err := reindexUTXOCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "getbalance":
		err := getBalanceCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "createblockchain":
		err := createBlockchainCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "listaddresses":
		err := listAddressesCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "createwallet":
		err := createWalletCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "send":
		err := sendCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress)
	}

	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockchainAddress)
	}

	if printChainCmd.Parsed() {
		cli.printChain()
	}

	if createWalletCmd.Parsed() {
		cli.createWallet()
	}

	if listAddressesCmd.Parsed() {
		cli.listAddresses()
	}

	if reindexUTXOCmd.Parsed() {
		cli.reindexUTXO()
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *sendAmount)
	}
}
