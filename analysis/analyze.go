package analysis

import (
	"io"

	"github.com/paralin/s2replay"
)

// AnalysisSchemaVersion identifies the record shapes Analyze writes.
const AnalysisSchemaVersion = 3

// EventSource yields replay events in order and io.EOF after the last; a
// *s2replay.Parser is one.
type EventSource interface {
	NextEvent() (s2replay.Event, error)
}

// AnalyzeOptions selects the optional parts of an analysis stream.
type AnalyzeOptions struct {
	// CombatWindows groups combat events into windows when set.
	CombatWindows *CombatWindowOptions
}

// Analyze reads events from src until io.EOF and passes each
// record to emit as soon as it closes: a header first, then inventory
// intervals, modifier intervals and combat windows, then the summary. It
// holds only the intervals and the window still open, so its memory does not
// grow with the length of the replay. Entity samples are counted but not
// emitted; the event stream carries them. The first error from emit stops the
// walk and is returned.
func Analyze(src EventSource, opts AnalyzeOptions, emit func(*AnalysisRecord) error) error {
	var emitErr error
	send := func(rec *AnalysisRecord) {
		if emitErr == nil {
			emitErr = emit(rec)
		}
	}
	send(&AnalysisRecord{Record: &AnalysisRecord_Header{Header: &AnalysisHeader{
		SchemaVersion: AnalysisSchemaVersion,
	}}})

	b := newBuilder(builderSinks{
		inventory: func(slot int32, iv InventoryInterval) {
			send(&AnalysisRecord{Record: &AnalysisRecord_Inventory{Inventory: inventoryRecord(slot, iv)}})
		},
		modifier: func(iv ModifierInterval) {
			send(&AnalysisRecord{Record: &AnalysisRecord_Modifier{Modifier: modifierRecord(iv)}})
		},
	})
	var windows *combatWindower
	if opts.CombatWindows != nil {
		windows = newCombatWindower(*opts.CombatWindows, func(w CombatWindow) {
			send(&AnalysisRecord{Record: &AnalysisRecord_CombatWindow{CombatWindow: combatWindowRecord(w)}})
		})
	}

	for index := 0; emitErr == nil; index++ {
		ev, err := src.NextEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		b.accept(ev)
		if windows != nil {
			windows.accept(ev, index)
		}
	}
	if emitErr != nil {
		return emitErr
	}

	b.finish()
	if windows != nil {
		windows.finish()
	}
	send(&AnalysisRecord{Record: &AnalysisRecord_Summary{Summary: summaryRecord(b.quality)}})
	return emitErr
}

func inventoryRecord(slot int32, iv InventoryInterval) *AnalysisInventory {
	return &AnalysisInventory{
		PlayerSlot: slot,
		StartTick:  iv.StartTick,
		EndTick:    iv.EndTick,
		StartTime:  iv.StartTime,
		EndTime:    iv.EndTime,
		OwnedItems: iv.OwnedItems,
	}
}

func modifierRecord(iv ModifierInterval) *AnalysisModifier {
	rec := &AnalysisModifier{
		StartTick:        iv.StartTick,
		EndTick:          iv.EndTick,
		StartTime:        iv.StartTime,
		EndTime:          iv.EndTime,
		LastObservedTick: iv.LastObservedTick,
		TableIndex:       iv.TableIndex,
		EntityId:         iv.EntityID,
		PlayerSlot:       iv.PlayerSlot,
		Parent:           iv.Parent,
		ModifierSubclass: iv.ModifierSubclass,
		SourceId:         iv.SourceID,
		Ability:          iv.Ability,
		Caster:           iv.Caster,
		StackCount:       iv.StackCount,
		Refreshes:        uint32(iv.Refreshes),
		Open:             iv.Open,
		PayloadProto:     iv.PayloadProto,
	}
	if iv.HasSerialNumber {
		rec.SerialNumber = &iv.SerialNumber
	}
	if iv.HasDuration {
		rec.Duration = &iv.Duration
	}
	if iv.HasLastAppliedTime {
		rec.LastAppliedTime = &iv.LastAppliedTime
	}
	return rec
}

func combatWindowRecord(w CombatWindow) *AnalysisCombatWindow {
	return &AnalysisCombatWindow{
		StartTick:       w.StartTick,
		EndTick:         w.EndTick,
		StartTime:       w.StartTime,
		EndTime:         w.EndTime,
		Events:          uint32(w.Events),
		DamageEvents:    uint32(w.DamageEvents),
		ModifierEvents:  uint32(w.ModifierEvents),
		PurchaseEvents:  uint32(w.PurchaseEvents),
		EntitySamples:   uint32(w.EntitySamples),
		FirstEventIndex: uint32(w.FirstEventIndex),
		LastEventIndex:  uint32(w.LastEventIndex),
		PlayerSlots:     w.PlayerSlots,
		Entities:        w.Entities,
		DamageAttackers: w.DamageAttackers,
		DamageVictims:   w.DamageVictims,
		ModifierParents: w.ModifierParents,
	}
}

func summaryRecord(q Quality) *AnalysisSummary {
	return &AnalysisSummary{
		Events:                     uint32(q.Events),
		InventoryTransitions:       uint32(q.InventoryTransitions),
		EntitySamples:              uint32(q.EntitySamples),
		ModifierAdds:               uint32(q.ModifierAdds),
		ModifierRefreshes:          uint32(q.ModifierRefreshes),
		ModifierRemoves:            uint32(q.ModifierRemoves),
		MissingPlayerSlotEvents:    uint32(q.MissingPlayerSlotEvents),
		UnmatchedModifierRefreshes: uint32(q.UnmatchedModifierRefreshes),
		UnmatchedModifierRemoves:   uint32(q.UnmatchedModifierRemoves),
		OpenModifierIntervals:      uint32(q.OpenModifierIntervals),
	}
}
