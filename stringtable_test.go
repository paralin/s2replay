package s2replay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"testing"

	"github.com/klauspost/compress/snappy"
)

// stringTableBitWriter encodes string-table entries bit by bit, mirroring the
// wire layout parseStringTable decodes.
type stringTableBitWriter struct {
	// buf contains complete encoded bytes.
	buf []byte
	// bits counts occupied low bits in the pending byte.
	bits uint8
	// value is the pending partial byte.
	value byte
}

// writeBits appends the low bits of value, least significant bit first.
func (w *stringTableBitWriter) writeBits(value uint32, bits uint8) {
	for range bits {
		w.value |= byte(value&1) << w.bits
		w.bits++
		value >>= 1
		if w.bits == 8 {
			w.buf = append(w.buf, w.value)
			w.bits = 0
			w.value = 0
		}
	}
}

// writeBytes appends each byte as an 8-bit value.
func (w *stringTableBitWriter) writeBytes(value []byte) {
	for _, b := range value {
		w.writeBits(uint32(b), 8)
	}
}

// writeUvarint32 appends value as a Valve uvarint.
func (w *stringTableBitWriter) writeUvarint32(value uint32) {
	for value >= 0x80 {
		w.writeBits(value&0x7f|0x80, 8)
		value >>= 7
	}
	w.writeBits(value, 8)
}

// bytes returns the encoded bits, flushing any partial final byte.
func (w *stringTableBitWriter) bytes() []byte {
	if w.bits != 0 {
		return append(w.buf, w.value)
	}
	return w.buf
}

// stringTableValue encodes a single entry that increments its index, omits
// the key, and carries value under the given byte count encoding.
func stringTableValue(value []byte, byteCount uint32, varint, compressed bool) []byte {
	// Write an entry header with optional value compression.
	var w stringTableBitWriter
	w.writeBits(1, 1) // increment index
	w.writeBits(0, 1) // no key
	w.writeBits(1, 1) // has value
	if compressed {
		w.writeBits(1, 1)
	}

	// Encode a variable-size value's byte count, or omit it for fixed-size values.
	if varint {
		if byteCount < 16 {
			w.writeBits(byteCount, 6)
		} else {
			w.writeBits(0x30|byteCount&0xf, 6)
			w.writeBits(byteCount>>4, 28)
		}
	} else if byteCount != 0 {
		w.writeBits(byteCount, 17)
	}

	// Append the payload and expose the final partial byte.
	w.writeBytes(value)
	return w.bytes()
}

// TestParseStringTableRejectsInvalidUpdateCounts checks the declared update
// count bounds before decoding starts.
func TestParseStringTableRejectsInvalidUpdateCounts(t *testing.T) {
	// Exercise negative, absolute-limit, and payload-relative counts.
	tests := []struct {
		name  string
		count int32
		want  error
	}{
		{name: "negative", count: -1, want: errInvalidStringTableUpdateCount},
		{name: "too large", count: maxStringTableUpdates + 1, want: errStringTableUpdateCountTooLarge},
		{name: "larger than payload", count: 9, want: errStringTableUpdateCountTooLarge},
	}

	// Reject each invalid declaration before decoding any item.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseStringTable([]byte{0}, tt.count, false, 0, 0, false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("parseStringTable() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestParseStringTableRejectsFixedUserDataBounds checks the fixed user-data
// bit count limits and truncation at the exact bit boundary.
func TestParseStringTableRejectsFixedUserDataBounds(t *testing.T) {
	// Fixed-size declarations must fit the configured entry bound.
	for _, bits := range []int32{-1, maxStringTableUserDataBytes*8 + 1} {
		_, err := parseStringTable([]byte{0}, 0, true, bits, 0, false)
		if err == nil {
			t.Fatalf("parseStringTable(userDataSizeBits=%d) succeeded", bits)
		}
	}

	// A legal fixed size still fails when the value payload is truncated.
	buf := stringTableValue(nil, 0, false, false)
	_, err := parseStringTable(buf, 1, true, 8, 0, false)
	if !errors.Is(err, errBitReadOverflow) {
		t.Fatalf("truncated fixed value error = %v, want %v", err, errBitReadOverflow)
	}
}

// TestParseStringTableAcceptsMaximumFixedUserData checks that a value at the
// Source user-data limit decodes intact.
func TestParseStringTableAcceptsMaximumFixedUserData(t *testing.T) {
	// Construct a value exactly at the accepted allocation boundary.
	value := bytes.Repeat([]byte{0xa5}, maxStringTableUserDataBytes)
	buf := stringTableValue(value, 0, false, false)
	items, err := parseStringTable(buf, 1, true, maxStringTableUserDataBytes*8, 0, false)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm both the entry count and decoded payload without indexing on failure.
	if len(items) != 1 {
		t.Fatalf("parsed item count = %d, want 1", len(items))
	}
	if !bytes.Equal(items[0].value, value) {
		t.Fatalf("parsed value length = %d, want %d", len(items[0].value), len(value))
	}
}

// TestParseStringTableRejectsOversizedVarintUserData checks the byte count
// limit for varint-encoded variable-length values.
func TestParseStringTableRejectsOversizedVarintUserData(t *testing.T) {
	buf := stringTableValue(nil, maxStringTableUserDataBytes+1, true, false)
	_, err := parseStringTable(buf, 1, false, 0, 0, true)
	if !errors.Is(err, errStringTableUserDataTooLarge) {
		t.Fatalf("parseStringTable() error = %v, want %v", err, errStringTableUserDataTooLarge)
	}
}

// TestParseStringTableRejectsOversizedKey checks the Source network-string
// key limit on an inline key.
func TestParseStringTableRejectsOversizedKey(t *testing.T) {
	var w stringTableBitWriter
	w.writeBits(1, 1)
	w.writeBits(1, 1)
	w.writeBits(0, 1)
	w.writeBytes(bytes.Repeat([]byte{'x'}, maxStringTableKeyBytes+1))
	w.writeBits(0, 8)
	w.writeBits(0, 1)
	_, err := parseStringTable(w.bytes(), 1, false, 0, 0, false)
	if !errors.Is(err, errStringTableKeyTooLarge) {
		t.Fatalf("parseStringTable() error = %v, want %v", err, errStringTableKeyTooLarge)
	}
}

// TestParseStringTableRejectsOversizedSnappyExpansion checks the decoded-size
// limit for a compressed value before decompression allocates.
func TestParseStringTableRejectsOversizedSnappyExpansion(t *testing.T) {
	compressed := snappy.Encode(nil, make([]byte, maxStringTableUserDataBytes+1))
	buf := stringTableValue(compressed, uint32(len(compressed)), true, true)
	_, err := parseStringTable(buf, 1, false, 0, 1, true)
	if !errors.Is(err, errStringTableUserDataTooLarge) {
		t.Fatalf("parseStringTable() error = %v, want %v", err, errStringTableUserDataTooLarge)
	}
}

// TestParseStringTableValidatesExplicitIndex checks the explicit entry index
// against the Source 20-bit index limit.
func TestParseStringTableValidatesExplicitIndex(t *testing.T) {
	// Cover the largest valid index and values that would exceed or wrap it.
	tests := []struct {
		name      string
		encoded   uint32
		wantIndex int32
		wantErr   error
	}{
		{name: "maximum valid", encoded: maxStringTableIndex - 1, wantIndex: maxStringTableIndex},
		{name: "first invalid", encoded: maxStringTableIndex, wantErr: errStringTableIndexTooLarge},
		{name: "uint32 maximum", encoded: ^uint32(0), wantErr: errStringTableIndexTooLarge},
	}

	// Decode each explicit index through the complete entry wire layout.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w stringTableBitWriter
			w.writeBits(0, 1)
			w.writeUvarint32(tt.encoded)
			w.writeBits(0, 1)
			w.writeBits(0, 1)

			items, err := parseStringTable(w.bytes(), 1, false, 0, 0, false)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseStringTable() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (len(items) != 1 || items[0].index != tt.wantIndex) {
				t.Fatalf("parseStringTable() items = %#v, want index %d", items, tt.wantIndex)
			}
		})
	}
}

// Sparse updates encode explicit gaps relative to the preceding entry. Numeric
// keys independently identify the target, as in the ActiveModifiers wire table.
func TestStringTableRelativeIndices(t *testing.T) {
	var wire []byte
	bit := 0
	put := func(value uint64, width int) {
		for i := range width {
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
