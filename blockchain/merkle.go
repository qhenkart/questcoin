package blockchain

import (
	"crypto/sha256"
	"log"
)

// MerkleTree is a system to simplify the process of verifying a transaction exists inside of a block without requiring the entire blockchain to confirm it
//
// this keeps every coin user from having the entire blockchain database on their computer
//
// By applying a merkle tree, we make a hash of each transaction, then add it up the tree until we reach the merkle root.
// The merkle root will be a hash that allows us to quickly verify the existence of the transactions
//
// Leafs of the tree must always be even. So if we only have 3 transactions, the 4th transaction would just be a copy of tx 3
//
//
// ||———————————————————————————————————————————————————————————————————————————————————————————————————————— ||
// || 																																																				||
// || 																						[Merkle Root] 																							||
// ||     										         	// 											     	  \\              											||
// || 																																																				||
// ||                 [sha256 Branch A+B]                                 [sha256 Branch C+D] 	  						||
// ||              //                     \\                          //                       \\ 						||
// || [Branch A: sha256 tx1]   [Branch B: sha256 tx2]    [Branch C: sha256 tx3]       [Branch D: sha256 tx4]  ||
// ————————————————————————————————————————————————————————————————————————————————————————————————————————————
//            |                      |                            |                           |
//--------------——————————————————————————————————————————————————————————————————————————————————————————————
//||   [transaction 1]         [transaction 2]            [transaction 3]                 [transaction 4]   || <- serialized transactions
//————————————————————————————————————————————————————————————————————————————————————————————————————————————
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleNode is a recursive tree structure
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte
}

// NewMerkleNode creates a new merkle node
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	// if the branches don't exist, make a hash and put it in the data field
	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		node.Data = hash[:]

		//otherwise we want to add branch A+B, hash it and apply it as the data
	} else {
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	return &node
}

// NewMerkleTree creates a new Merkle Tree
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	// pass in the transaction data and create the first branches
	//
	// the first set of branches do not have left and right branches. Only data (see illustration)
	for _, dat := range data {
		node := NewMerkleNode(nil, nil, dat)
		nodes = append(nodes, *node)
	}

	if len(nodes) == 0 {
		log.Panic("No merkel nodes")
	}
	// iterate through the nodes and connect them into the next branch

	// each node represents 2 transactions, so we know i should be the length of the transactions divided by 2

	for len(nodes) > 1 {
		// make sure that the leafs will be even, otherwise duplicate the last one
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		// create an array to represent levels of branches
		var level []MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			// left value will be i and right will be i+1
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			// add the node to the branch level
			level = append(level, *node)
		}
		// each iteration increases the level of nodes we are creating. Therefore each level would have half the amount of nodes as the previous level
		nodes = level
	}
	// pass the only node of the final iteration to be the root
	tree := MerkleTree{&nodes[0]}

	return &tree

}
