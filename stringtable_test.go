package s2replay

import (
	"bytes"
	"errors"
	"testing"

	"github.com/klauspost/compress/snappy"
)

type stringTableBitWriter struct {
	buf   []byte
	bits  uint8
	value byte
}

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

func (w *stringTableBitWriter) writeBytes(value []byte) {
	for _, b := range value {
		w.writeBits(uint32(b), 8)
	}
}

func (w *stringTableBitWriter) writeUvarint32(value uint32) {
	for value >= 0x80 {
		w.writeBits(value&0x7f|0x80, 8)
		value >>= 7
	}
	w.writeBits(value, 8)
}

func (w *stringTableBitWriter) bytes() []byte {
	if w.bits != 0 {
		return append(w.buf, w.value)
	}
	return w.buf
}

func stringTableValue(value []byte, byteCount uint32, varint, compressed bool) []byte {
	var w stringTableBitWriter
	w.writeBits(1, 1) // increment index
	w.writeBits(0, 1) // no key
	w.writeBits(1, 1) // has value
	if compressed {
		w.writeBits(1, 1)
	}
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
	w.writeBytes(value)
	return w.bytes()
}

func TestParseStringTableRejectsInvalidUpdateCounts(t *testing.T) {
	tests := []struct {
		name  string
		count int32
		want  error
	}{
		{name: "negative", count: -1, want: errInvalidStringTableUpdateCount},
		{name: "too large", count: maxStringTableUpdates + 1, want: errStringTableUpdateCountTooLarge},
		{name: "larger than payload", count: 9, want: errStringTableUpdateCountTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseStringTable([]byte{0}, tt.count, false, 0, 0, false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("parseStringTable() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseStringTableRejectsFixedUserDataBounds(t *testing.T) {
	for _, bits := range []int32{-1, maxStringTableUserDataBytes*8 + 1} {
		_, err := parseStringTable([]byte{0}, 0, true, bits, 0, false)
		if err == nil {
			t.Fatalf("parseStringTable(userDataSizeBits=%d) succeeded", bits)
		}
	}

	buf := stringTableValue(nil, 0, false, false)
	_, err := parseStringTable(buf, 1, true, 8, 0, false)
	if !errors.Is(err, errBitReadOverflow) {
		t.Fatalf("truncated fixed value error = %v, want %v", err, errBitReadOverflow)
	}
}

func TestParseStringTableAcceptsMaximumFixedUserData(t *testing.T) {
	value := bytes.Repeat([]byte{0xa5}, maxStringTableUserDataBytes)
	buf := stringTableValue(value, 0, false, false)
	items, err := parseStringTable(buf, 1, true, maxStringTableUserDataBytes*8, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !bytes.Equal(items[0].value, value) {
		t.Fatalf("parsed value length = %d, want %d", len(items[0].value), len(value))
	}
}

func TestParseStringTableRejectsOversizedVarintUserData(t *testing.T) {
	buf := stringTableValue(nil, maxStringTableUserDataBytes+1, true, false)
	_, err := parseStringTable(buf, 1, false, 0, 0, true)
	if !errors.Is(err, errStringTableUserDataTooLarge) {
		t.Fatalf("parseStringTable() error = %v, want %v", err, errStringTableUserDataTooLarge)
	}
}

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

func TestParseStringTableRejectsOversizedSnappyExpansion(t *testing.T) {
	compressed := snappy.Encode(nil, make([]byte, maxStringTableUserDataBytes+1))
	buf := stringTableValue(compressed, uint32(len(compressed)), true, true)
	_, err := parseStringTable(buf, 1, false, 0, 1, true)
	if !errors.Is(err, errStringTableUserDataTooLarge) {
		t.Fatalf("parseStringTable() error = %v, want %v", err, errStringTableUserDataTooLarge)
	}
}

func TestParseStringTableValidatesExplicitIndex(t *testing.T) {
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
