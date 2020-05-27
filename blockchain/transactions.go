package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

const miningReward = 100

// Transaction transactions do not have any identifiable information or secrets because they are public. They are just a collection
// of inputs and outputs and we can derive everything we need from those elements
type Transaction struct {
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

// SetID creates a hash based on the bytes that represent the transaction
func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encode := gob.NewEncoder(&encoded)
	err := encode.Encode(tx)
	handle(err)
	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

// CoinbaseTx creates the first genesis transaction
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}

	// referencing no output so it is missing data
	txin := TxInput{[]byte{}, -1, data}
	txout := TxOutput{miningReward, to}

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}
	tx.SetID()

	return &tx
}

// NewTransaction create a new transaction by accumulating the total amount of tokens a user has, validating it is less than what they want to send
// then iterate through all of the unused outputs and create new inputs for them.
//
// creates 2 new outputs. One is the amount being sent, the other is the amount not being sent
func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	// collect the accumulated total of coins and the output locations
	acc, validOutputs := chain.FindSpendableOutputs(from, amount)

	// if there is not enough funds, panic
	if acc < amount {
		log.Panic("Error: not enough funds")
	}

	// iterate through each valid output
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		handle(err)

		// iterate through each of the outs and create a new input for each unspent output that will be part of the transaction
		for _, out := range outs {
			input := TxInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	// create an output with the amount we are going to send and the address we are sending it to
	outputs = append(outputs, TxOutput{amount, to})

	// create a second output for the left over tokens that are not part of the transaction
	if acc > amount {
		outputs = append(outputs, TxOutput{acc - amount, from})
	}

	tx := Transaction{nil, inputs, outputs}
	tx.SetID()

	return &tx
}

// IsCoinbase checks whether the transaction is a coinbase transaction
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}
