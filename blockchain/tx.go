package blockchain

import (
	"bytes"
	"encoding/gob"

	"github.com/qhenkart/blockchain/wallet"
)

// TxOutput indivisible outputs, You cannot reference part of an output. eg. you can't take a $10 bill and split it in half to give change. You would have to make 2 new outputs with 5 each
type TxOutput struct {
	// value in tokens, assigned and locked in the output
	Value int
	// a value necessary to unlock the tokens locked in the value field
	//
	// in BTC this is implemented in Script lang
	PubKeyHash []byte
}

// TxOutputs defines a collection of outputs
type TxOutputs struct {
	Outputs []TxOutput
}

// TxInput refences to pevious outputs
type TxInput struct {
	// references the transaction that the output is inside of
	ID []byte
	// Index where the output appears. If the transaction has 3 outputs but we want to reference only 1. then we know transaction ID: x at index 2
	Out int
	// similiar to pubkey. Provides the data that is used in the outputs pubkey
	Signature []byte
	// public key that has not been hashed
	PubKey []byte
}

// NewTXOutput creates a new locked output
func NewTXOutput(value int, address string) *TxOutput {
	// create the output but ignore the key hash lock
	txo := &TxOutput{value, nil}
	// populate the pub key hash field by converting it into base58 bytes and locking it
	txo.Lock([]byte(address))
	return txo
}

// UsesKey checks to see if the input belongs to a public key
func (in *TxInput) UsesKey(pubKeyHash []byte) bool {
	// convert the input's public key to a hash
	lockingHash := wallet.PublicKeyHash(in.PubKey)
	return bytes.Compare(lockingHash, pubKeyHash) == 0
}

// Lock locks the output with an address
func (out *TxOutput) Lock(address []byte) {
	// turn the address into the public key hash
	pubKeyHash := wallet.Base58Decode(address)
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	// lock it. This gets deferred to UsesKey on the input of the next block
	out.PubKeyHash = pubKeyHash
}

// IsLockedWithKey checks if an output is locked with a provided key
func (out *TxOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Compare(out.PubKeyHash, pubKeyHash) == 0
}

// Serialize serializes a transaction into bytes
func (outs TxOutputs) Serialize() []byte {
	var buffer bytes.Buffer

	encode := gob.NewEncoder(&buffer)
	err := encode.Encode(outs)
	handle(err)

	return buffer.Bytes()
}

// DeserializeOutputs turns bytes into txoutputs
func DeserializeOutputs(data []byte) TxOutputs {
	var outputs TxOutputs

	decode := gob.NewDecoder(bytes.NewReader(data))
	err := decode.Decode(&outputs)
	handle(err)

	return outputs
}
