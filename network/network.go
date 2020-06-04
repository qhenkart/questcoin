package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"

	"github.com/qhenkart/blockchain/blockchain"
	"gopkg.in/vrecan/death.v3"
)

// Network Module
//
// Central Node: Localhost:3001
// // It is the node that all other nodes will connect to and it is the node that will send data between all of the other nodes
//
// Miner Node: stores the transactions in the memory pool. When there are enough transactions it will mine a new block
//
// Wallet Node: sends coins between wallets .
// // Unlike an SPV node, it will have a full copy of the block chain
//
// SPV: Simplified Payment Verification Node uses Merkle tree to verify transactions

//
// [Create Block] - > [Wallet connets and download blockchain] -> [miner connects and downloads blockchain] -> [wallet creates tx] -> [miner gets tx to memory pool] -> [enough tx -> mine block] ->
// [block sent to central node] -> [wallet syncs and verifies]
//
// Nodes interact with each other using RPCs (Remote Procedure Calls) -> see version struct
//
const (
	protocol      = "tcp"
	version       = 1
	commandLength = 12
	// the maximum amount of transactions that can exist in a block
	maxTxLimit = 2
)

var (
	// unique port for each instance
	nodeAddress string
	// unique port for the miner
	mineAddress string
	// KnownNodes contains all of the strings for the localhost addresses connected to this network
	KnownNodes = []string{"localhost:3001"}
	// blocks being sent from 1 client to another
	blocksInTransit = [][]byte{}
	// keep record of blockchain transactions
	memoryPool = make(map[string]blockchain.Transaction)
)

// Addr list of addresses that are connected to each of the nodes
//
// allows us to discover any nodes connected to other peers
type Addr struct {
	AddrList []string
}

// Block takes the address that the block is being built from and the block itself
//
// this allows us to identify where a block is coming from
type Block struct {
	AddrFrom string
	Block    []byte
}

// GetBlocks get the blocks from one node and send them to another
//
// calling this will fetch the block chain from one node and copy it to another node
type GetBlocks struct {
	AddrFrom string
}

// GetData get the active data from calling a block or a transaction and sending it from node to node
type GetData struct {
	AddrFrom string
	// could be a transaction or a block
	Type string
	ID   []byte
}

// Inv represents transactions or blocks
type Inv struct {
	AddrFrom string
	// transaction or block
	Type  string
	Items [][]byte
}

// Tx represents a transaction
type Tx struct {
	AddrFrom    string
	Transaction []byte
}

// Version Nodes communicate with each other via RPCs (Remote Procedure Calls).
//
// Version allows us to sync the blockchain between each of our nodes. When a server connects to each of our nodes, it sends it's version
// and receives the other nodes version.
//
// We are checking the height of the blockchain (whoever has the shortest block means they need to update their blockchain)
//
// Versipon is incremented based on how many blocks are in the chain.
type Version struct {
	Version int
	// the length of the actual chain (eg, chain is 4 blocks long)
	BestHeight int
	AddrFrom   string
}

// StartServer initializes the network. If there is no mineraddress then pass in an empty string
func StartServer(nodeID, minerAddress string) {
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)
	mineAddress = minerAddress

	ln, err := net.Listen(protocol, nodeAddress)
	if err != nil {
		log.Panic(err)
	}
	defer ln.Close()

	// the nodeID helps us identify which blockchain belongs to which client
	chain := blockchain.Continue(nodeID)
	defer chain.Database.Close()
	go CloseDB(chain)

	// if the node is not the central node. Then we want to request to get the most up to date information from the central node
	if nodeAddress != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Panic(err)
		}
		go HandleConnection(conn, chain)

	}
}

// MineTx goes through the blockchain. verifies each transaction from the memory pool.
// if all txs are verified, then we create a coinbasetx for the miner, and create a new block
func MineTx(chain *blockchain.Blockchain) {
	var txs []*blockchain.Transaction

	// take each tx from the memory pool and verify them
	for id := range memoryPool {
		fmt.Printf("tx: %s\n", memoryPool[id].ID)
		tx := memoryPool[id]
		if chain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		}
	}

	// if no txs were successfully verified then we know they are all invalid and should be ignored
	if len(txs) == 0 {
		fmt.Println("All Transactions are invalid")
		return
	}

	// create a new coinbase transaction with the miner address
	cbTx := blockchain.CoinbaseTx(mineAddress, "")
	// add the coinbase tx to the tx slice
	txs = append(txs, cbTx)

	// create a new block, add it to the UTXO and reindex
	newBlock := chain.MineBlock(txs)

	UTXOSet := blockchain.NewUTXOSet(chain)
	UTXOSet.Reindex()

	fmt.Println("New Block mined")

	// Delete all of the transactions from the memory pool now that they are part of the blockchain
	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	// send the new block to all of the known nodes
	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	//  if the memory pool still has items in it, we can recursively call MineTx
	if len(memoryPool) > 0 {
		MineTx(chain)
	}
}

// RequestBlocks iterates through the known nodes and requests blocks from each node
//
// it makes sure all of the blockchains are synced with one another
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
}

// CmdToBytes All commands are sent via bytes
// this takes a string and returns a slice of bytes
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}

	return bytes[:]
}

// BytesToCmd deserialize: converts bytes back to a cmd string
func BytesToCmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		// makes sure the byte is non zero. Removes spaces
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

// GobEncode serializes data into a slice of bytes
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

// NodeIsKnown checks to see if we have a  node recorded or not
func NodeIsKnown(addr string) bool {
	for _, node := range KnownNodes {
		if node == addr {
			return true
		}
	}

	return false
}

// CloseDB shuts down the database and quits the application in the event of a shutdown
func CloseDB(chain *blockchain.Blockchain) {
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
