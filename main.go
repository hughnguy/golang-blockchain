package main

import (
	"golang-blockchain/cli"
	"os"
)

func main() {
	defer os.Exit(0) // helps gracefully shutdown DB
	cmd := cli.CommandLine{}
	cmd.Run()
}
