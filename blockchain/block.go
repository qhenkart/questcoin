package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
	"time"
)

// Block represents a block on the blockchain. Including the Transactions, prev hash, current hash and nonce
//
// different peers will have different copies of these blocks.
type Block struct {
	Timestamp int64
	// current hash data block
	Hash         []byte
	Transactions []*Transaction
	// a hash that represents all of the previous blocks
	PrevHash []byte
	Nonce    int
	// index of the block in the chain, important for comparing blockchains with other peers
	Height int
}

// HashTransactions represent all transactions in a unique hash for PoW
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte

	// add each transaction to the 2d slice
	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.Serialize())
	}

	// create a merkle tree
	tree := NewMerkleTree(txHashes)

	// the root of the tree will serve as the unique identifier for each transaction
	return tree.RootNode.Data

}

// CreateBlock creates a block
func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{time.Now().Unix(), []byte{}, txs, prevHash, 0, height}
	// creates a new proof of work
	pow := NewProof(block)
	nonce, hash := pow.Run()

	// save the hash in the block
	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis creates the very first block in the blockchain. The genesis block will not have a previous data hash
func Genesis(coinbase *Transaction) *Block {
	// height of the genesis block is always zero
	return CreateBlock([]*Transaction{coinbase}, []byte{}, 0)
}

// Serialize turns a block into bytes
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	if err := encoder.Encode(b); err != nil {
		log.Panic(err)
	}
	return res.Bytes()
}

// Deserialize deserializes bytes into a block
func Deserialize(data []byte) *Block {
	var b Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	if err := decoder.Decode(&b); err != nil {
		log.Panic(err)
	}

	return &b
}

func handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
