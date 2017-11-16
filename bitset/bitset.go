package bitset

type BitSet struct {
	InternalSet  []byte
	Size int
}

func NewBitSet(size int) *BitSet {
	bitset := BitSet{Size: size, InternalSet: make([]byte, size/8)}
	return &bitset
}

func (bitset BitSet) Unset(indx int) {
	if indx < 0 {
		return
	}

	sliceIndex := indx / 8
	shift := uint32(indx % 8)
	// 128 = 0b10000000
	mask := 128 >> shift

	block := bitset.InternalSet[sliceIndex]
	block &^= byte(mask)
	bitset.InternalSet[sliceIndex] = block
}

func (bitset BitSet) Set(indx int) {
	if indx < 0 {
		return
	}

	sliceIndex := indx / 8
	shift := uint32(indx % 8)
	// 128 = 0b10000000
	mask := 128 >> shift

	bitsetBlock := bitset.InternalSet[sliceIndex]
	bitsetBlock |= byte(mask)
	bitset.InternalSet[sliceIndex] = bitsetBlock
}

func (bitset BitSet) Get(indx int) bool {
	block := bitset.InternalSet[indx/8]
	position := uint32(indx % 8)
	mask := 128 >> position
	return (block & byte(mask)) > 0
}
