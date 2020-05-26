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

// TxOutput indivisible outputs, You cannot reference part of an output. eg. you can't take a $10 bill and split it in half to give change. You would have to make 2 new outputs with 5 each
type TxOutput struct {
	// value in tokens, assigned and locked in the output
	Value int
	// a value necessary to unlock the tokens locked in the value field
	//
	// in BTC this is implemented in Script lang
	PubKey string
}

// TxInput refences to pevious outputs
type TxInput struct {
	// references the transaction that the output is inside of
	ID []byte
	// Index where the output appears. If the transaction has 3 outputs but we want to reference only 1. then we know transaction ID: x at index 2
	Out int
	// similiar to pubkey. Provides the data that is used in the outputs pubkey
	Sig string
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

// CanUnlock checks to see if the data matches the signature. If they come back as true,
// then the account owns the data inside the output referenced by the input
func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// CanBeUnlocked checks to see if the data matches the pubkey. If they come back as true,
// then the account owns the data inside the output
func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
