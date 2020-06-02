package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/qhenkart/blockchain/wallet"
)

const miningReward = 20

// Transaction transactions do not have any identifiable information or secrets because they are public. They are just a collection
// of inputs and outputs and we can derive everything we need from those elements
type Transaction struct {
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

// Serialize serializes a transaction into bytes
func (tx Transaction) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	if err := encoder.Encode(tx); err != nil {
		log.Panic(err)
	}
	return res.Bytes()
}

// Hash creates a hash from our transactions to use as the ID
func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

// CoinbaseTx creates the first genesis transaction
func CoinbaseTx(to, data string) *Transaction {
	// create something random to put in the coinbase data
	if data == "" {
		randData := make([]byte, 24)
		_, err := rand.Read(randData)
		handle(err)
		data = fmt.Sprintf("%x", randData)
	}

	// referencing no output so it is missing data
	txin := TxInput{[]byte{}, -1, nil, []byte(data)}
	txout := NewTXOutput(miningReward, to)

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}}
	tx.ID = tx.Hash()

	return &tx
}

// NewTransaction create a new transaction by accumulating the total amount of tokens a user has, validating it is less than what they want to send
// then iterate through all of the unused outputs and create new inputs for them.
//
// creates 2 new outputs. One is the amount being sent, the other is the amount not being sent
func NewTransaction(from, to string, amount int, UTXO *UTXOSet) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	wallets, err := wallet.CreateWallets()
	handle(err)
	w := wallets.GetWallet(from)
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	// collect the accumulated total of coins and the output locations
	acc, validOutputs := UTXO.FindSpendableOutputs(pubKeyHash, amount)

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
			input := TxInput{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	// create an output with the amount we are going to send and the address we are sending it to
	outputs = append(outputs, *NewTXOutput(amount, to))

	// create a second output for the left over tokens that are not part of the transaction
	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc-amount, from))
	}

	tx := Transaction{nil, inputs, outputs}
	// the id is now equal to the hashed version of all transactions
	tx.ID = tx.Hash()
	UTXO.Blockchain.SignTransaction(&tx, w.PrivateKey)

	return &tx
}

// IsCoinbase checks whether the transaction is a coinbase transaction
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// Sign signs and verifies transactions
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {
	// coinbase does not need to be signed
	if tx.IsCoinbase() {
		return
	}

	// we sign our transactions by the input. We use the inputs to access the referenced outputs
	// we need to iterate through all of the inputs to make sure they are valid
	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("the previous transaction does not exist")
		}
	}

	// creates a copy of the transaction
	txCopy := tx.TrimmedCopy()

	for inID, in := range txCopy.Inputs {
		prevTX := prevTXs[hex.EncodeToString(in.ID)]
		// set signature to nil to double check
		txCopy.Inputs[inID].Signature = nil
		// all of the transactions but the current one are empty, so these should be nil
		// thus each transaction is signed separately
		txCopy.Inputs[inID].PubKey = prevTX.Outputs[in.Out].PubKeyHash
		// serializes the transaction and hashes it
		// this is the data we want to sign
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inID].PubKey = nil

		//
		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inID].Signature = signature
	}
}

// TrimmedCopy creates a copy of a transaction without input signatures or keys
func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TxInput
	var outputs []TxOutput
	for _, in := range tx.Inputs {
		// copy each input sans the signature and key
		inputs = append(inputs, TxInput{in.ID, in.Out, nil, nil})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TxOutput{out.Value, out.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs}

	return txCopy
}

// Verify verifies if a transaction is valid
func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("Previous Transaction does not exist ")
		}
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	for inID, in := range tx.Inputs {
		prevTX := prevTXs[hex.EncodeToString(in.ID)]
		// set signature to nil to double check
		txCopy.Inputs[inID].Signature = nil
		// all of the transactions but the current one are empty, so these should be nil
		// thus each transaction is signed separately
		txCopy.Inputs[inID].PubKey = prevTX.Outputs[in.Out].PubKeyHash
		// serializes the transaction and hashes it
		// this is the data we want to sign
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inID].PubKey = nil

		// since each signature is just a pair of numbers and pub keys are a pair coordinates, we can deconstruct them
		r := big.Int{}
		s := big.Int{}
		sigLen := len(in.Signature)
		// r takes the last half
		r.SetBytes(in.Signature[:(sigLen / 2)])
		// s takes the first half
		s.SetBytes(in.Signature[(sigLen / 2):])

		x := big.Int{}
		y := big.Int{}
		keyLen := len(in.PubKey)
		// last half
		x.SetBytes(in.PubKey[:(keyLen / 2)])
		// first half
		y.SetBytes(in.PubKey[(keyLen / 2):])

		// create new public key
		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}
		// verify the public key with the ID and the signature
		if ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) == false {
			return false
		}
	}
	return true
}

// String converts the transaction into a formatted string for cli usage
func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))
	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("     Input %d:", i))
		lines = append(lines, fmt.Sprintf("       TXID:     %x", input.ID))
		lines = append(lines, fmt.Sprintf("       Out:       %d", input.Out))
		lines = append(lines, fmt.Sprintf("       Signature: %x", input.Signature))
		lines = append(lines, fmt.Sprintf("       PubKey:    %x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("     Output %d:", i))
		lines = append(lines, fmt.Sprintf("       Value:  %d", output.Value))
		lines = append(lines, fmt.Sprintf("       Script: %x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
