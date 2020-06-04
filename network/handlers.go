package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net"

	"github.com/qhenkart/blockchain/blockchain"
)

// HandleConnection reads a connection,
func HandleConnection(conn net.Conn, chain *blockchain.Blockchain) {
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

// HandleInv receives inventory payloads from other peers
func HandleInv(request []byte, chain *blockchain.Blockchain) {
	var payload Inv
	decodeData(request, &payload)

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
func HandleGetBlocks(request []byte, chain *blockchain.Blockchain) {
	var payload GetBlocks

	decodeData(request, &payload)
	// get all of the hashes from the blockchain
	blocks := chain.GetBlockHashes()
	// send the inventory with all of the block hashes
	//
	// if one of the blockchains doesn't have the same hashes, then they know they need to update it
	SendInv(payload.AddrFrom, "block", blocks)
}

// HandleGetData receives a request to send data back to a peer
func HandleGetData(request []byte, chain *blockchain.Blockchain) {
	var payload GetData

	decodeData(request, &payload)
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
func HandleTx(request []byte, chain *blockchain.Blockchain) {
	var payload Tx

	decodeData(request, &payload)

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

// HandleVersion decodes the version, calculates the best height, and compares it with the payload's best height.
// If ours is higher, then we need to send our version so they know to download our blockchain
// otherwise if their's is longer then we need to request for their blocks to update our blockchain
func HandleVersion(request []byte, chain *blockchain.Blockchain) {
	var payload Version

	decodeData(request, &payload)
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

// HandleBlock receives blocks from other peers and adds them to the blockchain
func HandleBlock(request []byte, chain *blockchain.Blockchain) {
	var payload Block

	decodeData(request, &payload)

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
		UTXOSet := blockchain.NewUTXOSet(chain)
		UTXOSet.Reindex()
	}
}

// HandleAddr recieves an address list from other peers and adds them to the known nodes
func HandleAddr(request []byte) {
	var payload Addr
	decodeData(request, &payload)
	// add the payloads address list to the known knowns
	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

func decodeData(in []byte, out interface{}) {
	var buff bytes.Buffer

	// extract the command
	buff.Write(in[commandLength:])

	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&out)
	if err != nil {
		log.Panic(err)
	}
}
