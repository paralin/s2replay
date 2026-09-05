package s2replay

import (
	"encoding/binary"
	"strconv"
	"testing"
)

// Sparse updates encode explicit gaps relative to the preceding entry. Numeric
// keys independently identify the target, as in the ActiveModifiers wire table.
func TestStringTableRelativeIndices(t *testing.T) {
	var wire []byte
	bit := 0
	put := func(value uint64, width int) {
		for i := 0; i < width; i++ {
			if bit/8 == len(wire) {
				wire = append(wire, 0)
			}
			wire[bit/8] |= byte((value>>i)&1) << (bit % 8)
			bit++
		}
	}
	indices := []int32{5, 6, 11, 802, 859}
	previous := int32(-1)
	for _, index := range indices {
		if index == previous+1 {
			put(1, 1)
		} else {
			put(0, 1)
			for _, b := range binary.AppendUvarint(nil, uint64(index-previous-2)) {
				put(uint64(b), 8)
			}
		}
		put(1, 1) // key present
		put(0, 1) // no key history
		for _, b := range []byte(strconv.Itoa(int(index)) + "\x00") {
			put(uint64(b), 8)
		}
		put(0, 1) // no value update
		previous = index
	}
	items, err := parseStringTable(wire, int32(len(indices)), false, 0, 0, false)
	if err != nil || len(items) != len(indices) {
		t.Fatalf("decode: %v, items=%d", err, len(items))
	}
	for i, item := range items {
		if item.index != indices[i] || item.key != strconv.Itoa(int(indices[i])) {
			t.Fatalf("entry %d: index=%d key=%q, want %d", i, item.index, item.key, indices[i])
		}
	}
}
