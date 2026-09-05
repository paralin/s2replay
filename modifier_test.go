package s2replay

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestModifierSerialReplacementAndPartialUpdates(t *testing.T) {
	for _, replacement := range []uint32{2, 0} {
		t.Run(map[uint32]string{2: "serial_two", 0: "explicit_zero"}[replacement], func(t *testing.T) {
			p := &Parser{clock: newClock(), modifiers: make(map[int32]modifierState), entityPlayerSlots: map[int32]int32{10: 1, 20: 2}}
			apply := func(tick uint32, entry *protocol.CModifierTableEntry) {
				t.Helper()
				data, err := entry.MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				p.clock.setTick(tick)
				if err := p.applyActiveModifierItem(tick, &stringTableItem{index: 7, value: data}); err != nil {
					t.Fatal(err)
				}
			}
			serial, parent, subclass := uint32(1), uint32(10), uint32(99)
			apply(1, &protocol.CModifierTableEntry{SerialNumber: &serial, Parent: &parent, ModifierSubclass: &subclass})
			// Omission must retain known serial1, otherwise replacement cannot be proven.
			apply(2, &protocol.CModifierTableEntry{Parent: &parent, ModifierSubclass: &subclass})
			parent = 20
			apply(3, &protocol.CModifierTableEntry{SerialNumber: &replacement, Parent: &parent, ModifierSubclass: &subclass})
			apply(4, &protocol.CModifierTableEntry{SerialNumber: &replacement, Parent: &parent, ModifierSubclass: &subclass})
			removed := protocol.MODIFIER_ENTRY_TYPE_MODIFIER_ENTRY_TYPE_REMOVED
			apply(5, &protocol.CModifierTableEntry{EntryType: &removed})
			want := []ModifierTransition{ModifierAdd, ModifierRefresh, ModifierRemove, ModifierAdd, ModifierRefresh, ModifierRemove}
			if len(p.pendingModifiers) != len(want) {
				t.Fatalf("transitions: %+v", p.pendingModifiers)
			}
			for i, transition := range want {
				if p.pendingModifiers[i].Transition != transition {
					t.Fatalf("event %d: %+v", i, p.pendingModifiers[i])
				}
			}
			old, next := p.pendingModifiers[2], p.pendingModifiers[3]
			if old.Tick != 3 || old.SerialNumber != 1 || old.Parent != 10 || !old.MatchedPrior {
				t.Fatalf("old removal: %+v", old)
			}
			if next.Tick != 3 || next.SerialNumber != replacement || next.Parent != 20 || next.MatchedPrior {
				t.Fatalf("new add: %+v", next)
			}
			if p.pendingModifiers[1].SerialNumber != 1 || p.pendingModifiers[5].SerialNumber != replacement {
				t.Fatalf("partial serials: %+v", p.pendingModifiers)
			}
			if len(p.modifiers) != 0 {
				t.Fatal("removed occupant retained")
			}
			if len(p.pendingEvents) != len(want) || p.pendingEvents[2].PlayerSlot != 1 || p.pendingEvents[3].PlayerSlot != 2 {
				t.Fatalf("event attribution: %+v", p.pendingEvents)
			}
		})
	}
}

func TestModifierUnknownSerialDoesNotInventReplacement(t *testing.T) {
	p := &Parser{clock: newClock(), modifiers: make(map[int32]modifierState)}
	parent, serial := uint32(10), uint32(2)
	for i, entry := range []*protocol.CModifierTableEntry{{Parent: &parent}, {Parent: &parent, SerialNumber: &serial}} {
		data, err := entry.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.applyActiveModifierItem(uint32(i+1), &stringTableItem{index: 7, value: data}); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.pendingModifiers) != 2 || p.pendingModifiers[1].Transition != ModifierRefresh {
		t.Fatalf("unproven replacement: %+v", p.pendingModifiers)
	}
}

// TestModifierPayloadMergeContract pins the generated codec used for instance deltas.
func TestModifierPayloadMergeContract(t *testing.T) {
	truth, falsity := true, false
	one, zero := int32(1), int32(0)
	x, y := float32(3), float32(4)
	prior := &protocol.CModifierTableEntry{Bool1_: &truth, Int1_: &one, Vec1_: &protocol.CMsgVector{X: &x}}
	update := &protocol.CModifierTableEntry{Bool1_: &falsity, Int1_: &zero, Vec1_: &protocol.CMsgVector{Y: &y}}
	data, err := update.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	unknown := binary.AppendUvarint(nil, 1000<<3)
	unknown = binary.AppendUvarint(unknown, 7)
	data = append(data, unknown...)
	merged := prior.CloneVT()
	for range 2 {
		if err := merged.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}
	if merged.Bool1_ == nil || *merged.Bool1_ || merged.Int1_ == nil || *merged.Int1_ != 0 || merged.Bool2_ != nil || merged.Vec1_.X == nil || *merged.Vec1_.X != x || merged.Vec1_.Y == nil || *merged.Vec1_.Y != y || merged.Vec1_.Z != nil {
		t.Fatal("generated merge lost presence or nested components")
	}
	if !*prior.Bool1_ || *prior.Int1_ != 1 || prior.Vec1_.Y != nil {
		t.Fatal("merging a clone mutated prior payload")
	}
	encoded, err := merged.CloneVT().MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(encoded, bytes.Repeat(unknown, 2)) {
		t.Fatal("generated merge must retain, not interpret or deduplicate, unknown wire occurrences")
	}
}
