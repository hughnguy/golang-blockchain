package testutils

import (
	"fmt"
	"golang-blockchain/wallet"
)

func RandomAddress() string {
	w := wallet.MakeWallet()
	return fmt.Sprintf("%s", w.Address())
}
