package s2replay

import "testing"

func TestAbilitySlotWireEncoding(t *testing.T) {
	decoder := findFieldDecoder(&field{fieldType: &fieldType{baseType: "EAbilitySlots_t"}})
	for _, tc := range []struct {
		wire byte
		want uint32
	}{
		{0x00, 0}, {0x02, 1}, {0x04, 2}, {0x06, 3}, {0x0e, 7}, {0x2e, 23}, {0x01, 0xffff},
	} {
		got, err := decoder(newPacketReader([]byte{tc.wire}))
		if err != nil || got != tc.want {
			t.Fatalf("wire %02x: got %v, %v; want %d", tc.wire, got, err, tc.want)
		}
	}
	if _, err := decoder(newPacketReader([]byte{0x03})); err == nil {
		t.Fatal("accepted invalid negative slot")
	}
	if _, err := decoder(newPacketReader(nil)); err == nil {
		t.Fatal("accepted missing enum bytes")
	}
	// Ordinary unsigned fields with the same byte retain their unsigned value.
	plain := findFieldDecoder(&field{fieldType: &fieldType{baseType: "uint16"}})
	got, err := plain(newPacketReader([]byte{0x0e}))
	if err != nil || got != uint32(14) {
		t.Fatalf("unsigned decoder changed: %v, %v", got, err)
	}
}
