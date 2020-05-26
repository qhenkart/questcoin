package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
)

// Block ...
type Block struct {
	Hash         []byte
	Transactions []*Transaction
	PrevHash     []byte
	Nonce        int
}

//// DeriveHash unused since we are deriving the hash in the  the PoW
//func (b *Block) DeriveHash() {
//info := bytes.Join([][]byte{b.Data, b.PrevHash}, []byte{})
//hash := sha256.Sum256(info)
//b.Hash = hash[:]
//}

// HashTransactions represent all transactions in a unique hash for PoW
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	// add each transaction to the 2d slice
	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.ID)
	}

	// concat all the bytes together and hash them
	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{}))

	return txHash[:]

}

// CreateBlock ...
func CreateBlock(txs []*Transaction, prevHash []byte) *Block {
	block := &Block{[]byte{}, txs, prevHash, 0}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis ...
func Genesis(coinbase *Transaction) *Block {
	return CreateBlock([]*Transaction{coinbase}, []byte{})
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
