package s2replay

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

// TestParseStringTableRejectsIncrementPastLimit checks the implicit index path.
func TestParseStringTableRejectsIncrementPastLimit(t *testing.T) {
	// Place one entry at the last allowed index, then increment it.
	var w stringTableBitWriter
	w.writeBits(0, 1)
	w.writeUvarint32(maxStringTableIndex - 1)
	w.writeBits(0, 2)
	w.writeBits(1, 1)
	w.writeBits(0, 2)

	// Both explicit and implicit indexes must obey the same limit.
	_, err := parseStringTable(w.bytes(), 2, false, 0, 0, false)
	if !errors.Is(err, errStringTableIndexTooLarge) {
		t.Fatalf("increment past limit: got %v, want %v", err, errStringTableIndexTooLarge)
	}
}

// TestCreateStringTableRejectsOversizedExpansion checks the allocation guard
// through the parser's decoded-message dispatcher.
func TestCreateStringTableRejectsOversizedExpansion(t *testing.T) {
	// A Snappy header can declare a large result without carrying that payload.
	p := &Parser{stringTables: newStringTables()}
	compressed := true
	msg := &protocol.CSVCMsg_CreateStringTable{
		Name:           proto("bounded"),
		DataCompressed: &compressed,
		StringData:     binary.AppendUvarint(nil, maxStringTableDataBytes+1),
	}

	// Reject the declared allocation before trying to decode the missing block.
	if err := p.applyDecodedMessage(1, msg); !errors.Is(err, errStringTableDataTooLarge) {
		t.Fatalf("oversized table expansion: got %v, want %v", err, errStringTableDataTooLarge)
	}
}

// TestUpdateStringTablePropagatesInvalidCount checks that invalid updates fail
// instead of appearing to succeed at the decoded-message boundary.
func TestUpdateStringTablePropagatesInvalidCount(t *testing.T) {
	// Target an existing table so the malformed count reaches the entry decoder.
	p := &Parser{stringTables: newStringTables()}
	table := p.stringTables.getOrCreate("bounded")
	count := int32(-1)
	msg := &protocol.CSVCMsg_UpdateStringTable{
		TableId:           &table.index,
		NumChangedEntries: &count,
	}

	// Preserve the decoder's specific error through dispatch.
	if err := p.applyDecodedMessage(1, msg); !errors.Is(err, errInvalidStringTableUpdateCount) {
		t.Fatalf("invalid update count: got %v, want %v", err, errInvalidStringTableUpdateCount)
	}
}

// TestParseStringTableRejectsMissingUpdates checks truncation at a byte boundary.
func TestParseStringTableRejectsMissingUpdates(t *testing.T) {
	// Eight empty entries occupy exactly three bytes, with no padding bits.
	var w stringTableBitWriter
	for range 8 {
		w.writeBits(1, 1)
		w.writeBits(0, 2)
	}

	// A declared ninth entry must not silently become a partial success.
	_, err := parseStringTable(w.bytes(), 9, false, 0, 0, false)
	if !errors.Is(err, errBitReadOverflow) {
		t.Fatalf("missing update: got %v, want %v", err, errBitReadOverflow)
	}
}
