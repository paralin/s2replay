package s2replay

import (
	"bytes"
	"testing"
)

// TestFixed8Decoder verifies that a field tagged with the "fixed8" encoder
// reads exactly one byte and never continues into a following varint byte.
// Build 10932 moved most small integer and enum fields to this encoder.
func TestFixed8Decoder(t *testing.T) {
	decoder := findFieldDecoder(&field{
		encoder:   "fixed8",
		fieldType: &fieldType{baseType: "uint8"},
	})
	// 0x80 has the high bit set. A varint decoder would consume a second byte;
	// fixed8 must stop after one byte and leave the next byte for the following
	// field.
	r := newPacketReader([]byte{0x80, 0x01})
	got, err := decoder(r)
	if err != nil || got != uint32(0x80) {
		t.Fatalf("fixed8 uint8: got %v, %v; want 128", got, err)
	}
	next, err := r.readByte()
	if err != nil || next != 0x01 {
		t.Fatalf("fixed8 consumed past one byte: next=%v err=%v", next, err)
	}

	// Signed fixed8 fields are sign-extended.
	signed := findFieldDecoder(&field{
		encoder:   "fixed8",
		fieldType: &fieldType{baseType: "int8"},
	})
	got, err = signed(newPacketReader([]byte{0xff}))
	if err != nil || got != int32(-1) {
		t.Fatalf("fixed8 int8: got %v, %v; want -1", got, err)
	}

	// A field without the encoder keeps its variable-length integer encoding.
	plain := findFieldDecoder(&field{fieldType: &fieldType{baseType: "uint8"}})
	got, err = plain(newPacketReader([]byte{0x80, 0x01}))
	if err != nil || got != uint32(128) {
		t.Fatalf("varint uint8 changed: got %v, %v; want 128", got, err)
	}
}

// TestBinaryBlockDecoder verifies that CUtlBinaryBlock reads a uvarint32 byte
// length followed by that many raw bytes.
func TestBinaryBlockDecoder(t *testing.T) {
	decoder := findFieldDecoder(&field{fieldType: &fieldType{baseType: "CUtlBinaryBlock"}})
	got, err := decoder(newPacketReader([]byte{0x03, 0xaa, 0xbb, 0xcc, 0x99}))
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := got.([]byte); !ok || !bytes.Equal(b, []byte{0xaa, 0xbb, 0xcc}) {
		t.Fatalf("binary block: got %v; want aa bb cc", got)
	}
}
