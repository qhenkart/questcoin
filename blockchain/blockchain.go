package blockchain

import (
	"encoding/hex"
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
func (chain *BlockChain) AddBlock(transactions []*Transaction) {
	var lastHash []byte
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

// FindUnspentTransactions measuring outputs that have no input references then they are "unspent" tokens. By counting all of the
// unspent outputs that are associated with a certain user, we can tell how many tokens a user owns
func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspentTxs []Transaction

	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		// iterate through each transaction inside of a block
		// encode each id into a hexadecimal string
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

			// label so we can break just to this portion of the loop
		Outputs:
			for outIdx, out := range tx.Outputs {
				// if the output is inside the map then iterate to find the matching index
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						// find the matching Id
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				// check to see if the address can unlock the transaction
				if out.CanBeUnlocked(address) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}

			// skip coinbase transactions since they have no inputs
			if !tx.IsCoinbase() {
				// interate through the transactions inputs to find more outputs by input reference
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		// checks to see if the block is the genesis block
		if len(block.PrevHash) == 0 {
			break
		}
	}
	return unspentTxs
}

// FindUTXO find all unspent transactions outputs for a single address (user)
func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var UTXOs []TxOutput
	unspentTransactions := chain.FindUnspentTransactions(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}
	return UTXOs
}

// FindSpendableOutputs accumulates the total unspent outputs as well as their addresses to sent a specified amount
func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			// make sure a transaction cannot be made where a user does not have enough tokens
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)
				// once there is enough accumulated, we can break out of the loop
				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOuts
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
