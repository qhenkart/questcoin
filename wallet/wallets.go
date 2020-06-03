package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

const walletFile = "./tmp/wallets_%s.data"

// Wallets creates a rudamentory database structure and avoid mixing with the block chain badger db
type Wallets struct {
	Wallets map[string]*Wallet
}

// CreateWallets reads from disc to initialize and populate wallets
func CreateWallets(nodeID string) (*Wallets, error) {
	wallets := Wallets{}

	wallets.Wallets = make(map[string]*Wallet)

	err := wallets.LoadFile(nodeID)
	return &wallets, err
}

// AddWallet creates a new wallet and adds it to the wallets structure
func (ws *Wallets) AddWallet() string {
	wallet := MakeWallet()
	address := fmt.Sprintf("%s", wallet.Address())

	ws.Wallets[address] = wallet
	return address
}

// GetAllAddresses returns an array of wallet addresses (aka map keys)
func (ws Wallets) GetAllAddresses() []string {
	var addresses []string
	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}

	return addresses

}

// GetWallet retrieves a single wallet based on the address
func (ws Wallets) GetWallet(address string) Wallet {
	return *ws.Wallets[address]
}

// LoadFile loads the wallets from disc
func (ws *Wallets) LoadFile(nodeID string) error {
	walletFile := fmt.Sprintf(walletFile, nodeID)
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
	if err := decoder.Decode(&wallets); err != nil {
		log.Panic(err)
	}

	ws.Wallets = wallets.Wallets
	return nil
}

// SaveFile saves the wallets to disc
func (ws *Wallets) SaveFile(nodeID string) {
	var content bytes.Buffer
	walletFile := fmt.Sprintf(walletFile, nodeID)

	gob.Register(elliptic.P256())
	encoder := gob.NewEncoder(&content)
	if err := encoder.Encode(ws); err != nil {
		log.Panic(err)
	}

	if err := ioutil.WriteFile(walletFile, content.Bytes(), 0644); err != nil {
		log.Panic(err)
	}
}
