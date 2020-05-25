package blockchain

import (
	"fmt"
	"log"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"
)

// BlockChain ...
//type BlockChain struct {
//Blocks []*Block
//}
type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// Iterator ...
type Iterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Init ..
func Init() *BlockChain {
	//return &BlockChain{[]*Block{Genesis()}}
	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)

	db, err := badger.Open(opts)
	handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte("lh"))
		if err == badger.ErrKeyNotFound {
			fmt.Println("No existing blockchain found, Adding Genesis")
			genesis := Genesis()
			fmt.Println("Genesis proved")

			// serialize genesis
			err = txn.Set(genesis.Hash, genesis.Serialize())
			handle(err)

			// set the hash to the last hash
			err := txn.Set([]byte("lh"), genesis.Hash)

			lastHash = genesis.Hash

			return err
		}

		item, err := txn.Get([]byte("lh"))
		handle(err)

		lastHash = valueHash(item)
		return err

	})
	handle(err)

	//create new block chain in memory
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// AddBlock ...
func (chain *BlockChain) AddBlock(data string) {
	//prevBlock := chain.Blocks[len(chain.Blocks)-1]
	//new := CreateBlock(data, prevBlock.Hash)
	//chain.Blocks = append(chain.Blocks, new)

	var lastHash []byte
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		handle(err)

		lastHash = valueHash(item)

		return err
	})

	handle(err)

	newBlock := CreateBlock(data, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		handle(err)

		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})

	handle(err)
}

// Iterator ...
func (chain *BlockChain) Iterator() *Iterator {
	return &Iterator{chain.LastHash, chain.Database}
}

// Next loops to retrieve the previous hash
func (iter *Iterator) Next() *Block {
	var b *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		// retrieve the last block
		item, err := txn.Get(iter.CurrentHash)
		handle(err)

		// get the hash representation of the block
		encodedBlock := valueHash(item)
		// deserialize it into our block struct
		b = Deserialize(encodedBlock)
		return err
	})

	handle(err)

	// update the currentHash field for looping
	iter.CurrentHash = b.PrevHash
	return b
}

// valueHash shortcut method to quickly retrieve the hash value from a db item
func valueHash(item *badger.Item) []byte {
	var hash []byte
	err := item.Value(func(val []byte) error {
		hash = append([]byte{}, val...)
		return nil
	})

	if err != nil {
		log.Panic(err)
	}
	return hash
}
