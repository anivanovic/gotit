package bitset

import "testing"

func TestSet(t *testing.T) {
	bitset := NewBitSet(24)
	bitset.Set(5)
	t.Logf("%b", bitset.InternalSet)
	bitset.Set(6)
	t.Logf("%b", bitset.InternalSet)
	bitset.Set(5)
	t.Logf("%b", bitset.InternalSet)
	bitset.Set(22)
	t.Logf("%b", bitset.InternalSet)

	bitset.Unset(5)
	t.Logf("%b", bitset.InternalSet)
}

func TestGet(t *testing.T) {
	bitset := NewBitSet(24)
	bitset.Set(5)
	bitset.Set(6)
	t.Log(bitset.Get(5))
	t.Log(bitset.Get(2))
	t.Log(bitset.Get(6))
}

func TestFirstUnset(t *testing.T) {
	bitset := NewBitSet(16)
	bitset.Set(0)
	bitset.Set(1)
	bitset.Set(2)
	bitset.Set(3)
	firstUnset := bitset.FirstUnset(0)
	if firstUnset != 4 {
		t.Error("Unset index ", firstUnset, "Should be 4")
	}

	bitset = NewBitSet(25)
	firstUnset = bitset.FirstUnset(0)
	if firstUnset != 0 {
		t.Error("Unset index ", firstUnset, "Should be 0")
	}
}
