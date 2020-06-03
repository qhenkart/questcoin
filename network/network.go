package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
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

// SendAddr send an address from one peer to another
func SendAddr(address string) {
	nodes := Addr{KnownNodes}
	// nodeAddress is the node address of the client that is connecting
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := GobEncode(nodes)
	// prepend the command to the payload
	request := append(CmdToBytes("addr"), payload...)

	SendData(address, request)
}

// SendBlock sends a block from one peer to another
func SendBlock(addr string, b *blockchain.Block) {
	// create the block to send and seralize the actual block
	data := Block{nodeAddress, b.Serialize()}

	// convert it to bytes
	payload := GobEncode(data)

	//prepend the command in bytes
	request := append(CmdToBytes("block"), payload...)

	SendData(addr, request)
}

// SendInv sends inventory from one peer to another
func SendInv(address, kind string, items [][]byte) {
	// create structure
	inventory := Inv{nodeAddress, kind, items}
	// convert it to bytes
	payload := GobEncode(inventory)
	// prepend the command
	request := append(CmdToBytes("inv"), payload...)

	SendData(address, request)
}

// SendTx sends a transaction from one peer to another
func SendTx(addr string, tnx *blockchain.Transaction) {
	data := Tx{nodeAddress, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(addr, request)
}

// SendVersion calculates the best height and sends the version from one peer to another
func SendVersion(addr string, chain *blockchain.BlockChain) {
	// Checks to see what the length of the blockchain actually is
	bestHeight := chain.GetBestHeight()
	payload := GobEncode(Version{version, bestHeight, nodeAddress})

	request := append(CmdToBytes("version"), payload...)

	SendData(addr, request)
}

// SendGetBlocks requests blocks from another peer
func SendGetBlocks(address string) {
	payload := GobEncode(GetBlocks{nodeAddress})
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(address, request)
}

// SendGetData requests a set of data from another peer
func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(address, request)
}

// SendData sends data from one node to another
func SendData(addr string, data []byte) {
	// connect to the interent via tcp
	conn, err := net.Dial(protocol, addr)
	if err != nil {
		fmt.Printf("%s is not available\n", addr)
		var updatedNodes []string

		// if the node is unavailable, we need to update the available nodes
		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}

		KnownNodes = updatedNodes

		return
	}

	defer conn.Close()

	// call the connection and copy the data into the connection
	_, err = io.Copy(conn, bytes.NewReader(data))
	if err != nil {
		log.Panic(err)
	}
}

// HandleAddr recieves an address list from other peers and adds them to the known nodes
func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	// extract the command
	buff.Write(request[commandLength:])

	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// add the payloads address list to the known knowns
	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

// HandleBlock receives blocks from other peers and adds them to the blockchain
func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	block := blockchain.Deserialize(blockData)

	fmt.Println("Recevied a new block!")
	chain.AddBlock(block)

	fmt.Printf("Added block %x\n", block.Hash)

	// check to see how many blocks are in transit. If there are more, then request the next blocks from other peers if there are any
	if len(blocksInTransit) > 0 {
		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		// skips the zeroth index since we just read the first one
		blocksInTransit = blocksInTransit[1:]
	} else {
		// otherwise reindex the UTXO set
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()
	}
}

// HandleInv receives inventory payloads from other peers
func HandleInv(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inv

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Recevied inventory with %d %s\n", len(payload.Items), payload.Type)

	// if the payload type is a block. then add them to the blocks in transit
	if payload.Type == "block" {
		blocksInTransit = payload.Items

		// take the first items block hash
		blockHash := payload.Items[0]

		// request the block from other peers
		SendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		for _, b := range blocksInTransit {
			// add new blocks in transit
			if bytes.Compare(b, blockHash) != 0 {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit = newInTransit
	}

	// if the payload is a transaction. then see if we have the transaction in our memory pool, otherwise request to get the transaction
	if payload.Type == "tx" {
		txID := payload.Items[0]

		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(payload.AddrFrom, "tx", txID)
		}
	}
}

// HandleGetBlocks receives a request to send blocks back to a peer
func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// get all of the hashes from the blockchain
	blocks := chain.GetBlockHashes()
	// send the inventory with all of the block hashes
	//
	// if one of the blockchains doesn't have the same hashes, then they know they need to update it
	SendInv(payload.AddrFrom, "block", blocks)
}

// HandleGetData receives a request to send data back to a peer
func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// if the payload type is a block, retrieve the block from the blockchain based on the payload id
	if payload.Type == "block" {
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}
		// send the block to the other peers so they can download it
		SendBlock(payload.AddrFrom, &block)
	}

	//if the payload type is a transaction, add it to the memory pool and send the transaction to the other peers so they can keep track of it
	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTx(payload.AddrFrom, &tx)
	}
}

// HandleTx receives requests for transactions. Our wallet will be sending transactions to our miner and central node
func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	txData := payload.Transaction
	tx := blockchain.DeserializeTransaction(txData)

	// add the transaction or our memory pool
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	fmt.Printf("%s, %d", nodeAddress, len(memoryPool))

	// check to see if the node address is the central node. If it is the central node
	// it has the responsibility to update the other nodes
	if nodeAddress == KnownNodes[0] {
		// then iterate through each known node address and send the transaction to all of the nodes (except for the current node and the sender's node)
		for _, node := range KnownNodes {
			if node != nodeAddress && node != payload.AddrFrom {
				// for all the non central nodes and non miner nodes
				SendInv(node, "tx", [][]byte{tx.ID})
			}
		}
		// for miner nodes. Check the memory pool length. If we have more transactions than 2, then we want to mine a new transaction
	} else {
		if len(memoryPool) >= maxTxLimit && len(mineAddress) > 0 {
			// verify the transactions and mine a new block
			MineTx(chain)
		}
	}
}

// MineTx goes through the blockchain. verifies each transaction from the memory pool.
// if all txs are verified, then we create a coinbasetx for the miner, and create a new block
func MineTx(chain *blockchain.BlockChain) {
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
	UTXOSet := blockchain.UTXOSet{chain}
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

// HandleVersion decodes the version, calculates the best height, and compares it with the payload's best height.
// If ours is higher, then we need to send our version so they know to download our blockchain
// otherwise if their's is longer then we need to request for their blocks to update our blockchain
func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// calculate best height
	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	// if theirs is larger, then we need to request their blocks to update our blockchain
	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)

		// if ours is larger then send our version so they know to update their blockchain with our blocks
	} else if bestHeight > otherHeight {
		SendVersion(payload.AddrFrom, chain)
	}

	// add the incoming address to the known nodes if it isn't already there
	if !NodeIsKnown(payload.AddrFrom) {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
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

// HandleConnection reads a connection,
func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}

	// pull out the command and convert it to a string
	command := BytesToCmd(req[:commandLength])
	fmt.Printf("Received %s command\n", command)

	// handle the connection based on the command
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
func CloseDB(chain *blockchain.BlockChain) {
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
