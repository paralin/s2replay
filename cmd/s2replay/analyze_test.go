package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paralin/s2replay"
)

const analyzeGoldenPath = "testdata/analyze_golden.json"

func TestAnalyzeOutputGolden(t *testing.T) {
	events := []s2replay.Event{
		{
			Type:       s2replay.EventPurchase,
			Tick:       1,
			GameTime:   1,
			PlayerSlot: 2,
			OwnedItems: []uint32{10},
			Purchase:   &s2replay.PurchaseEvent{Tick: 1, GameTime: 1, PlayerSlot: 2, AbilityID: 10},
		},
		{
			Type:       s2replay.EventEntitySample,
			Tick:       2,
			GameTime:   2,
			Entity:     100,
			PlayerSlot: 2,
			OwnedItems: []uint32{10},
			EntitySample: &s2replay.EntitySample{
				ClassID:     8,
				ClassName:   "CCitadelPlayerPawn",
				Health:      700,
				MaxHealth:   900,
				HasHealth:   true,
				HasPosition: true,
			},
		},
		{
			Type:       s2replay.EventModifier,
			Tick:       3,
			GameTime:   3,
			Entity:     100,
			PlayerSlot: 2,
			Modifier: &s2replay.ModifierEvent{
				Transition:       s2replay.ModifierAdd,
				TableIndex:       7,
				Parent:           100,
				ModifierSubclass: 20,
				AbilitySubclass:  10,
			},
		},
		{
			Type:       s2replay.EventModifier,
			Tick:       5,
			GameTime:   5,
			Entity:     100,
			PlayerSlot: 2,
			Modifier: &s2replay.ModifierEvent{
				Transition:       s2replay.ModifierRemove,
				TableIndex:       7,
				Parent:           100,
				ModifierSubclass: 20,
				AbilitySubclass:  10,
			},
		},
		{Type: s2replay.EventDamage, Tick: 6, GameTime: 6, Entity: 100, PlayerSlot: 2},
	}
	out, err := analysisOutputFromEvents(events, 8, "damage,modifier")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("S2REPLAY_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(analyzeGoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(analyzeGoldenPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(analyzeGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("analyze golden mismatch; rerun with S2REPLAY_UPDATE_GOLDEN=1 after verifying the schema")
	}
}

func TestAnalyzeRejectsUnknownCombatEvent(t *testing.T) {
	if _, err := analysisOutputFromEvents(nil, 8, "damage,bogus"); err == nil {
		t.Fatal("unknown combat event type should fail")
	}
}
