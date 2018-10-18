// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package kademlia

import (
	"container/heap"
	"math/big"

	"storj.io/storj/pkg/dht"
	"storj.io/storj/pkg/pb"
)

//We've encapsulated our priorityQueue{} implementation (below) so that you don't
//have to know how the Golang heap API works (nor our priority XOR logic)

//XorQueue is a priority queue where the priority is key XOR distance
type XorQueue struct {
	pq *priorityQueue
}

//NewXorQueue returns a priorityQueue with priority based on XOR from targetBytes
func NewXorQueue(nodes []*pb.Node, target dht.NodeID) XorQueue {
	targetBytes := new(big.Int).SetBytes(target.Bytes())
	pq := make(priorityQueue, len(nodes))
	for i, node := range nodes {
		pq[i] = &item{value: node, index: i}
		pq[i].priority = new(big.Int).Xor(targetBytes, new(big.Int).SetBytes([]byte(node.GetId())))
	}
	heap.Init(&pq)
	return XorQueue{pq: &pq} 
}

//Insert adds Node onto the queue
func (x XorQueue) Insert(node *pb.Node, target dht.NodeID) {
	targetBytes := new(big.Int).SetBytes(target.Bytes())
	heap.Push(x.pq, &item{
		value:    node,
		priority: new(big.Int).Xor(targetBytes, new(big.Int).SetBytes([]byte(node.GetId()))),
	})
}

//PopClosest removed the closest priority node from the queue
func (x XorQueue) PopClosest() (*pb.Node, dht.NodeID) {
	item := *(heap.Pop(x.pq).(*item))
	return item.value, item.priority
}

//Len returns the length of the queue
func (x XorQueue) Len() int {
	return x.pq.Len()
}

// Resize resizes the queue, keeping the closest k items
func (x *XorQueue) Resize(k int) {
	oldPq := x.pq
	x.pq = &priorityQueue{}
	for i := 0; i < k && len(*oldPq) > 0; i++ {
		item := heap.Pop(oldPq)
		heap.Push(x.pq, item)
	}
	heap.Init(x.pq)
}



// An item is something we manage in a priority queue.
type item struct {
	value    *pb.Node // The value of the item; arbitrary.
	priority *big.Int // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A priorityQueue implements heap.Interface and holds items.
type priorityQueue []*item

// Len returns the length of the priority queue
func (pq priorityQueue) Len() int { return len(pq) }

// Less does what you would think
func (pq priorityQueue) Less(i, j int) bool {
	// this sorts the nodes where the node popped has the closest location
	return pq[i].priority.Cmp(pq[j].priority) < 0
}

// Swap swaps two ints
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

// Push adds an item to the top of the queue
// must call heap.fix to resort
func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*item)
	item.index = n
	*pq = append(*pq, item)
}

// Pop returns the item with the lowest priority
func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}