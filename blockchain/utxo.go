package blockchain

import (
	"bytes"
	"encoding/hex"
	"log"

	"github.com/dgraph-io/badger"
)

var (
	// badger does not have any tables, so to get around that, we can create a key prefix to separate them from other items
	utxoPrefix   = []byte("utxo-")
	prefixLength = len(utxoPrefix)
)

// UTXOSet allows us to access the database connected to our blockchain
//
// we can create a new layer in our db that has just hte UTXOs (unspent transactions)
type UTXOSet struct {
	Blockchain *BlockChain
}

// FindSpendableOutputs accumulates the total unspent outputs as well as their addresses to sent a specified amount
func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	accumulated := 0

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// iterate through each prefix key
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v := valueHash(item)

			k = bytes.TrimPrefix(k, utxoPrefix)
			txID := hex.EncodeToString(k)

			// get the outputs of the id
			outs := DeserializeOutputs(v)

			// iterate through transaction outputs
			for outIdx, out := range outs.Outputs {
				// make sure a transaction cannot be made where a user does not have enough tokens
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOuts[txID] = append(unspentOuts[txID], outIdx)
				}
			}
		}
		return nil
	})

	handle(err)
	return accumulated, unspentOuts
}

// Reindex clears out the database of utxos, and rebuild the set directly from the blockchain
func (u UTXOSet) Reindex() {
	// alias the db
	db := u.Blockchain.Database

	// remove all items in the database with this prefix
	u.DeleteByPrefix(utxoPrefix)

	// collect all unspent outputs from the blockchain
	UTXO := u.Blockchain.FindUTXO()

	err := db.Update(func(txn *badger.Txn) error {
		// iterate through all utxos
		for txID, outs := range UTXO {
			// decode the index into bytes
			key, err := hex.DecodeString(txID)
			if err != nil {
				return err
			}
			// add prefix
			key = append(utxoPrefix, key...)

			// add it to the database
			err = txn.Set(key, outs.Serialize())
			handle(err)
		}

		return nil
	})
	handle(err)
}

// Update takes a block and uses it to update the utxo set
func (u *UTXOSet) Update(block *Block) {
	db := u.Blockchain.Database

	err := db.Update(func(txn *badger.Txn) error {
		// iterate through each transaction
		for _, tx := range block.Transactions {
			if !tx.IsCoinbase() {
				// iterate through each input
				for _, in := range tx.Inputs {
					// create an output for each input
					updatedOuts := TxOutputs{}
					// take the id of the input and add the prefix to it
					inID := append(utxoPrefix, in.ID...)
					// get the value of the input from the db
					item, err := txn.Get(inID)
					handle(err)
					v := valueHash(item)

					// deserialixe the output value
					outs := DeserializeOutputs(v)

					// iterate through each output
					for outIdx, out := range outs.Outputs {
						// if the output is not attached to the input then we know it is unspent.. add it to the updated outputs
						if outIdx != in.Out {
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					if len(updatedOuts.Outputs) == 0 {
						// if there are no unspent outputs, then get rid of the utxo transaction ids
						if err := txn.Delete(inID); err != nil {
							log.Panic(err)
						}
					} else {
						// save the unspent outputs with the utxo prefixed transaction id
						if err := txn.Set(inID, updatedOuts.Serialize()); err != nil {
							log.Panic(err)
						}
					}
				}
			}

			// account for coinbase transactions in the block, they will always be unspent
			newOutputs := TxOutputs{}
			for _, out := range tx.Outputs {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}

			txID := append(utxoPrefix, tx.ID...)
			if err := txn.Set(txID, newOutputs.Serialize()); err != nil {
				log.Panic(err)
			}
		}

		return nil
	})
	handle(err)
}

// FindUnspentTransactions measuring outputs that have no input references then they are "unspent" tokens. By counting all of the
// unspent outputs that are associated with a certain user, we can tell how many tokens a user owns
func (u UTXOSet) FindUnspentTransactions(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		// iterate through UTXOS prefixes
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			// get the value of each utxo prefixed item
			v := valueHash(it.Item())
			outs := DeserializeOutputs(v)

			// iterate through each output, check to see if it is locked by the provided hash address
			for _, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})
	handle(err)

	return UTXOs
}

// CountTransactions counts how many unspent transactions exist within the set
func (u UTXOSet) CountTransactions() int {
	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// iterate only through the utxo keys
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}
		return nil
	})
	handle(err)

	return counter
}

// DeleteByPrefix goes through the db and deletes the prefix keys by bulk
func (u *UTXOSet) DeleteByPrefix(prefix []byte) {
	// create closure that has all of the deleted keys
	deleteKeys := func(keysForDelete [][]byte) error {
		// access db via the blockchain connection and expose the badger transaction
		if err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
			// iterate through the 2d slice of bytes
			for _, k := range keysForDelete {
				// delete each key
				if err := txn.Delete(k); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	// this is the optimal amount of keys to delete at a time
	// deletes in 100,000 increments
	collectSize := 100000

	// open read only transaction
	u.Blockchain.Database.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		// allows us to read the keys but without the values for optimization
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		keysForDelete := make([][]byte, 0, collectSize)
		keysCollected := 0

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// create a copy of the key
			key := it.Item().KeyCopy(nil)
			// assign the key for deletion
			keysForDelete = append(keysForDelete, key)
			keysCollected++

			// if we hit the limit, then delete the first 100,000 keys set for deletion
			if keysCollected == collectSize {
				if err := deleteKeys(keysForDelete); err != nil {
					log.Panic(err)
				}

				// reset the array and the counter
				keysForDelete = make([][]byte, 0, collectSize)
				keysCollected = 0
			}
		}

		// if the keys are above 0 but below 100,000 at the end of the loop, delete the rest
		if keysCollected > 0 {
			if err := deleteKeys(keysForDelete); err != nil {
				log.Panic(err)
			}
		}
		return nil
	})
}
