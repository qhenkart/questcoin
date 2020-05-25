package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
)

// Block ...
type Block struct {
	Hash     []byte
	Data     []byte
	PrevHash []byte
	Nonce    int
}

//// DeriveHash unused since we are deriving the hash in the  the PoW
//func (b *Block) DeriveHash() {
//info := bytes.Join([][]byte{b.Data, b.PrevHash}, []byte{})
//hash := sha256.Sum256(info)
//b.Hash = hash[:]
//}

// CreateBlock ...
func CreateBlock(data string, prevHash []byte) *Block {
	block := &Block{[]byte{}, []byte(data), prevHash, 0}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis ...
func Genesis() *Block {
	return CreateBlock("genesis", []byte{})
}

// Serialize ...
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	if err := encoder.Encode(b); err != nil {
		log.Panic(err)
	}
	return res.Bytes()
}

// Deserialize ..
func Deserialize(data []byte) *Block {
	var b Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	if err := decoder.Decode(&b); err != nil {
		log.Panic(err)
	}

	return &b
}

func handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
