package blockchain

import "github.com/dgraph-io/badger"

// Iterator Creates a cursor for the blockchain that traverses the blockchain in reverse (starting from the last block)
type Iterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Iterator creates an iterator for the blockchain. The chain iterates backwards
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
