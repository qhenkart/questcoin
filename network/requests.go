package network

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/qhenkart/blockchain/blockchain"
)

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
func SendVersion(addr string, chain *blockchain.Blockchain) {
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
