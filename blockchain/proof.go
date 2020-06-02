package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/big"
)

// take data from block

// create a counter (nonce) starts at 0

// create a hash of the data plus the counter

// check the hash to see if it meets a set of requirements

// Requirements:
// The first few bytes must contain 0s
//    -- as difficulty goes up, there must be more 0s

// Difficulty normally there would be an algorithm that increases the difficulty over a long period of time, to account for
// a growing number of minors as well as the increase in computation power of computers in general
//
// the goal is to make the amount of time to mine a block to be about the same
const Difficulty = 12

// ProofOfWork ...
type ProofOfWork struct {
	Block  *Block
	Target *big.Int
}

// NewProof ...
func NewProof(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	// 256 is the number of bytes inside of the hash
	// left shift
	target.Lsh(target, uint(256-Difficulty))

	pow := &ProofOfWork{b, target}

	return pow
}

// InitData takes the previous hash and the hashed transaction, combines them together
func (pow *ProofOfWork) InitData(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.Block.PrevHash,
			pow.Block.HashTransactions(),
			ToHex(int64(nonce)),
			ToHex(int64(Difficulty)),
		},
		[]byte{},
	)

	return data
}

// Run ...
func (pow *ProofOfWork) Run() (int, []byte) {
	var intHash big.Int
	var hash [32]byte

	nonce := 0

	// run forever (virtually)
	for nonce < math.MaxInt64 {
		data := pow.InitData(nonce)
		hash = sha256.Sum256(data)

		fmt.Printf("\r%x", hash)

		intHash.SetBytes(hash[:])

		if intHash.Cmp(pow.Target) == -1 {
			// less than the target we are looking for. Block is signed
			break
		} else {
			nonce++
		}
	}

	fmt.Println()
	return nonce, hash[:]
}

// Validate ...
func (pow *ProofOfWork) Validate() bool {
	var intHash big.Int
	data := pow.InitData(pow.Block.Nonce)

	hash := sha256.Sum256(data)
	intHash.SetBytes(hash[:])

	return intHash.Cmp(pow.Target) == -1
}

// ToHex ...
func ToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}
