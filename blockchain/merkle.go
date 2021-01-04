package blockchain

import (
	"crypto/sha256"
	"log"
)

// Can use root of merkle tree to quickly verify if a transaction is inside a block
// Used by light nodes to be able to check if a block contains a transaction without actually downloading the block itself
type MerkleTree struct {
	RootNode *MerkleNode
}

type MerkleNode struct {
	Left *MerkleNode
	Right *MerkleNode
	Data []byte
}

// A node in the merkle tree contains the data of its 2 children appended together and sha256 hashed
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	// create empty node
	node := MerkleNode{}

	if left == nil && right == nil { // is a leaf node (has no children)
		hash := sha256.Sum256(data)
		node.Data = hash[:] // if no children, then data will just be a hash of the data passed down
	} else { // otherwise take children and generate a hash
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:] // set current node's data field as a hash of its children's data
	}

	// set the children of this node (both can be nil if this node is a leaf)
	node.Left = left
	node.Right = right

	return &node
}

// Start with bottom nodes to create merkle tree and eventually create a root node at the top level
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	// start at bottom of tree, iterate all group of bytes and create a node per group
	// bottom branches do not have left and right children so pass in null nodes
	for _, dat := range data {
		node := NewMerkleNode(nil, nil, dat)
		nodes = append(nodes, *node)
	}

	if len(nodes) == 0 {
		log.Panic("No merkel nodes")
	}

	// iterate through nodes until reaching top level (when there is only 1 node left)
	for len(nodes) > 1 {
		if len(nodes)%2 != 0 { // if # of nodes on previous level is odd length, then concatenate the last node again to make it even
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		// nodes on this current level. start off with empty list
		var level []MerkleNode

		// iterate each pair of bottom level nodes in order to create the parent node
		for i := 0; i < len(nodes); i += 2 { // increment by 2 since we're using each set of 2 children to create parent data
			// get the two nodes on the previous bottom level iterated, to create parent node on current level
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			// add this new parent to nodes in current level
			level = append(level, *node)
		}
		// assign nodes on this level to main nodes array. nodes array keeps track of nodes on previous bottom level
		// this main nodes array contains the nodes on the level below. maybe rename it to: children []MerkleNode?
		// eventually the nodes array will contain the top level root node
		nodes = level
	}

	// add root node to tree
	tree := MerkleTree{&nodes[0]}

	return &tree
}
