package bitset

import "testing"

func TestSet(t *testing.T) {
	bitset := NewBitSet(24)
	bitset.Set(5)
	t.Logf("%b", bitset.internal)
	bitset.Set(6)
	t.Logf("%b", bitset.internal)
	bitset.Set(5)
	t.Logf("%b", bitset.internal)
	bitset.Set(22)
	t.Logf("%b", bitset.internal)

	bitset.Unset(5)
	t.Logf("%b", bitset.internal)
}

func TestGet(t *testing.T) {
	bitset := NewBitSet(24)
	bitset.Set(5)
	bitset.Set(6)
	t.Log(bitset.Get(5))
	t.Log(bitset.Get(2))
	t.Log(bitset.Get(6))
}
