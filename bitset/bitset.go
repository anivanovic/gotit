package bitset

import "fmt"

type BitSet struct {
	internal []byte
	Size     int
}

func NewBitSet(size int) *BitSet {
	bitset := BitSet{Size: size, internal: make([]byte, size/8)}
	return &bitset
}

func (bitset BitSet) Unset(indx int) {
	if indx < 0 {
		return
	}

	sliceIndex := indx / 8
	shift := uint32(indx % 8)
	mask := 128 // 0b10000000
	mask = mask >> shift

	fmt.Printf("%b", mask)
	block := bitset.internal[sliceIndex]
	block &^= byte(mask)
	bitset.internal[sliceIndex] = block
}

func (bitset BitSet) Set(indx int) {
	if indx < 0 {
		return
	}

	sliceIndex := indx / 8
	shift := uint32(indx % 8)
	mask := 128 // 0b10000000
	fmt.Printf("mask before %b\n", mask)
	mask = mask >> shift

	fmt.Printf("mask after %b\n", mask)
	bitsetBlock := bitset.internal[sliceIndex]
	fmt.Printf("block before %b\n", bitsetBlock)
	bitsetBlock |= byte(mask)
	fmt.Printf("block after %b\n", bitsetBlock)

	bitset.internal[sliceIndex] = bitsetBlock
}

func (bitset BitSet) Get(indx int) bool {
	block := bitset.internal[indx/8]
	position := uint32(indx % 8)
	mask := 128 >> position
	return (block & byte(mask)) > 0
}
