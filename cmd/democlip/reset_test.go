package main

import (
	"testing"

	"github.com/paralin/s2replay/protocol"
)

// TestResetEntitiesKeepsMembershipStreamsSeparate checks that a retained
// entity between two active indices cannot consume a base-frame slot.
func TestResetEntitiesKeepsMembershipStreamsSeparate(t *testing.T) {
	// Decode the wire packet rather than inspecting the inputs to its writer.
	packet := resetEntities([]int32{1, 900}, []int32{4, 31}, 1363, 1364, 3072)
	var frame protocol.CSVCMsg_PacketEntities
	eachMsg(packet, func(kind uint32, body []byte) {
		if kind == uint32(protocol.SVC_Messages_svc_PacketEntities) {
			if err := frame.UnmarshalVT(body); err != nil {
				t.Fatal(err)
			}
		}
	})

	// The clock and frame base are independent; only the active entries are inline.
	if frame.GetDeltaFrom() != 1363 || frame.GetServerTick() != 1364 || frame.GetUpdatedEntries() != 2 {
		t.Fatal("wrong reset tick, base, or inline count")
	}
	inline := bitReader{buf: frame.GetEntityData()}
	if inline.ubitvar() != 1 || inline.bits(2) != 3 || inline.ubitvar() != 898 || inline.bits(2) != 3 {
		t.Fatal("inline removals must leave and delete indices 1 and 900")
	}

	// The retained indices must be cleared, not added to the next frame.
	deferred := frame.GetNonTransmittedEntities()
	if deferred == nil || deferred.GetHeaderCount() != 2 {
		t.Fatal("wrong non-transmitted clear count")
	}
	dormant := bitReader{buf: deferred.GetData()}
	if dormant.ubitvar() != 4 || dormant.bits(1) != 0 || dormant.ubitvar() != 26 || dormant.bits(1) != 0 {
		t.Fatal("non-transmitted updates must clear indices 4 and 31")
	}
}

// TestRewriteDoesNotUseClockAsFrameBase checks a clock-only tail after a reset.
func TestRewriteDoesNotUseClockAsFrameBase(t *testing.T) {
	// Write a frame, then a clock-only packet with a higher tick.
	var frame uint32
	identity := func(tick uint32) uint32 { return tick }
	rewrite(resetEntities(nil, nil, 36, 39, 3072), keepAll, identity, &frame)
	body, err := (&protocol.CNETMsg_Tick{Tick: ptr(uint32(42))}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	var clock bitWriter
	clock.ubitvar(uint32(protocol.NET_Messages_net_Tick))
	clock.varint(uint32(len(body)))
	for _, b := range body {
		clock.bits(uint32(b), 8)
	}

	// A newer net_Tick cannot replace the last actual entity frame.
	rewrite(clock.finish(), keepAll, identity, &frame)
	if frame != 39 {
		t.Fatalf("frame = %d, want 39", frame)
	}
}

// TestBaselineSlotsKeepClassValuesWithTheirKeys checks that a changed table
// order does not put one class's encoded values into another class's slot.
func TestBaselineSlotsKeepClassValuesWithTheirKeys(t *testing.T) {
	// Give each source the same keys in a different order and distinct data.
	var order baselineOrder
	first := &protocol.CDemoStringTables{Tables: []*protocol.CDemoStringTablesTableT{{
		TableName: ptr(classBaselineTable),
		Items: []*protocol.CDemoStringTablesItemsT{
			{Str: ptr("778"), Data: []byte{1}}, {Str: ptr("775"), Data: []byte{2}},
		},
	}}}
	second := &protocol.CDemoStringTables{Tables: []*protocol.CDemoStringTablesTableT{{
		TableName: ptr(classBaselineTable),
		Items: []*protocol.CDemoStringTablesItemsT{
			{Str: ptr("775"), Data: []byte{3}}, {Str: ptr("778"), Data: []byte{4}},
		},
	}}}
	original := second.Tables[0]

	// Check both the class identity and the new value in each fixed slot.
	if order.align(first) || !order.align(second) {
		t.Fatal("only the second snapshot should be reordered")
	}
	items := second.Tables[0].Items
	if items[0].GetStr() != "778" || items[0].Data[0] != 4 || items[1].GetStr() != "775" || items[1].Data[0] != 3 {
		t.Fatal("baseline values did not follow the class keys")
	}
	if original.Items[0].GetStr() != "775" {
		t.Fatal("align modified the source table")
	}
}
