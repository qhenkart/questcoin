package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	// we can have multiple dbs for each of our nodes and they are represented in differrent folders
	dbPath      = "./tmp/blocks_%s"
	genesisData = "First Transaction from Genesis"
)

// BlockChain defines the blockchain and database access for the node
type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// checks to see if the database exists or not
func dbExists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}

	return true
}

// Init Initializes the database,
//
// initializes the blockchain with the first genesis block and first coinbase transaction
func Init(address, nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)

	if dbExists(path) {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := openDB(path, opts)
	handle(err)

	var lastHash []byte
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
func Continue(nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)
	if !dbExists(path) {
		fmt.Println("No existing blockchain found, must be initialized first ")
		runtime.Goexit()
	}

	var lastHash []byte
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := openDB(path, opts)
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

// GetBestHeight retrieves the last (best) height
func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		// get the last hash
		item, err := txn.Get([]byte("lh"))
		handle(err)
		lastHash := valueHash(item)

		// last block from the last hash
		item, err = txn.Get(lastHash)
		handle(err)
		lastBlockData := valueHash(item)

		lastBlock = *Deserialize(lastBlockData)

		return nil
	})
	handle(err)

	// return lastblock height
	return lastBlock.Height
}

// GetBlock retrieves a block based on a block hash from the blockchain
func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	if err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(blockHash)
		if err != nil {
			return errors.New("Block is not found")
		}

		// get the block and assign it to the closure
		blockData := valueHash(item)
		block = *Deserialize(blockData)

		return nil
	}); err != nil {
		return block, err
	}

	return block, nil
}

// GetBlockHashes retrieves all of the block hashes from the blockchain
func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		// adds the block hash
		blocks = append(blocks, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

// AddBlock takes a block ptr and adds it to the blockchain if it doesn't already exist
func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {
		// if the block is already in the db, skip
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData := block.Serialize()
		// add the block to the db
		err := txn.Set(block.Hash, blockData)
		handle(err)

		// get the last hash
		item, err := txn.Get([]byte("lh"))
		handle(err)
		lastHash := valueHash(item)

		// get the last block from the lasthash
		item, err = txn.Get(lastHash)
		handle(err)
		lastBlockData := valueHash(item)

		lastBlock := Deserialize(lastBlockData)

		// compare the block height with the last block height
		// if it is larger, then set the new block to the last hash
		if block.Height > lastBlock.Height {
			err = txn.Set([]byte("lh"), block.Hash)
			handle(err)
			chain.LastHash = block.Hash
		}

		return nil
	})
	handle(err)
}

// MineBlock adds a block to the block chain.
// pulls the last hash from the database, creates a new block with the transaction history and the last hash
// then adds the new block into the database and updates the lasthash key with the latest block
func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	for _, tx := range transactions {
		if chain.VerifyTransaction(tx) != true {
			log.Panic("Invalid Transaction")
		}
	}

	err := chain.Database.View(func(txn *badger.Txn) error {
		// get the last hash
		item, err := txn.Get([]byte("lh"))
		handle(err)
		lastHash = valueHash(item)

		// use the last hash to get the last block
		item, err = txn.Get(lastHash)
		handle(err)
		lastBlockData := valueHash(item)

		lastBlock := Deserialize(lastBlockData)

		// get the last height from the last block
		lastHeight = lastBlock.Height

		return err
	})

	handle(err)

	// increment the last height in the block
	newBlock := CreateBlock(transactions, lastHash, lastHeight+1)

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

// retry if we corrupt the data in the database, we can retrieve the data and rebuild
//
// if there is a lock file in the database folder, then the retry function is called
//
// it deletes an un-garbage collected lock file and reopens the database
func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}

// openDB opens the database, if there is an error due to a lockfile, we call the retry function to get rid of the lock file and restore database access
func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	db, err := badger.Open(opts)
	if err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	}
	return db, nil
}
