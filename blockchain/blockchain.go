package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"
	// verify if the blockchain db exists, created by badger
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction From Genesis"
)

// BlockChain ...
type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// Iterator ...
type Iterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// checks to see if the database exists or not
func dbExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}

	return true
}

// Init Initializes the database,
//
// initializes the blockchain with the first genesis block and first coinbase transaction
func Init(address string) *BlockChain {
	var lastHash []byte

	if dbExists() {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil
	db, err := badger.Open(opts)
	handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		// address will be the first miner who gets the first reward
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)

		fmt.Println("Genesis created")
		err = txn.Set(genesis.Hash, genesis.Serialize())
		handle(err)

		// set the hash to the last hash
		err := txn.Set([]byte("lh"), genesis.Hash)
		lastHash = genesis.Hash
		return err

	})
	handle(err)

	//create new block chain in memory
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// Continue continues the blockchain when the coinbase and genesis have already been initialized
func Continue(address string) *BlockChain {
	if !dbExists() {
		fmt.Println("No existing blockchain found, must be initialized first ")
		runtime.Goexit()
	}

	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil
	db, err := badger.Open(opts)
	handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		handle(err)
		lastHash = valueHash(item)
		return nil
	})

	chain := BlockChain{lastHash, db}
	return &chain
}

// AddBlock adds a block to the block chain.
// pulls the last hash from the database, creates a new block with the transaction history and the last hash
// then adds the new block into the database and updates the lasthash key with the latest block
func (chain *BlockChain) AddBlock(transactions []*Transaction) *Block {
	var lastHash []byte

	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		handle(err)

		lastHash = valueHash(item)

		return err
	})

	handle(err)

	newBlock := CreateBlock(transactions, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		handle(err)

		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})

	handle(err)

	return newBlock
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

// FindUTXO find all unspent transactions outputs and return a map of unspent transaction outputs organized by transaction id
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		// iterate through each block from reverse, starting with the very last unspent transactions and continueing up the chain
		block := iter.Next()
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			// iterate through each output
			for outIdx, out := range tx.Outputs {
				// if a transaction id exists in the spent transactions array iterate through the indexes to see if there is a match
				// if there is a match then we know it's a spent output and we can skip it
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				// take the entire map into the outs variable that matches the transaction id
				outs := UTXO[txID]
				// put each output into the txOutputs value of the map
				outs.Outputs = append(outs.Outputs, out)
				// put the updated TXoutputs back into the map
				UTXO[txID] = outs
			}

			if !tx.IsCoinbase() {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					// inputs reference used outputs by index, so add inputs to the spentTXOs map so we can match them in the next (previous) block
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return UTXO
}

// FindTransaction finds a transaction in the block chain
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		// for each transaction, compare the transaction id with the passed id
		//
		// if there is a match. return the transaction
		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction does not exist")
}

// SignTransaction ...
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		// add each previous transaction to the prvious transaction map
		prevTX, err := chain.FindTransaction(in.ID)
		handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	// sign the previous key and previous transactions
	tx.Sign(privKey, prevTXs)
}

// VerifyTransaction verifies each previous transaction
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		// add each previous transaction to the prvious transaction map
		prevTX, err := chain.FindTransaction(in.ID)
		handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	// sign the previous key and previous transactions
	return tx.Verify(prevTXs)

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
