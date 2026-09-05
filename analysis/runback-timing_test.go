package analysis

import (
	"math"
	"testing"

	"github.com/paralin/s2replay"
)

// TestRunbackTimerPairsPreserveProvenance checks both equipment projections.
func TestRunbackTimerPairsPreserveProvenance(t *testing.T) {
	// Build real ownership with independently observed cooldown and recharge fields.
	pawn := runbackPawn(100, 92, 1)
	item := runbackSample(100, 300, 21, "CCitadel_Item_Empty", -1)
	item.HasOwnerEntity = true
	item.OwnerEntity = pawn.Entity
	item.OwnerEntitySerial = pawn.EntitySerial
	item.HasCooldownStart, item.CooldownStart, item.CooldownStartTick = true, 0, 80
	item.HasCooldownEnd, item.CooldownEnd, item.CooldownEndTick = true, 14.5, 81
	item.HasChargeRechargeStart, item.ChargeRechargeStart, item.ChargeRechargeStartTick = true, 9.25, 82
	item.HasChargeRechargeEnd, item.ChargeRechargeEnd, item.ChargeRechargeEndTick = true, float32(math.NaN()), 83
	ability := item
	ability.Entity = 301
	ability.ClassName = "CCitadel_Ability_Dash"
	ability.HasChargeRechargeEnd = false
	facts, err := buildRunbackFacts([]s2replay.EntitySample{pawn, item, ability}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Keep the two projections semantically identical for their known timer fields.
	hero := facts.Heroes[0]
	for _, timer := range []RunbackFloat{hero.Items[0].CooldownStart, hero.Abilities[0].CooldownStart} {
		if !timer.Present {
			t.Fatal("zero start became missing")
		}
		if timer.Value != 0 {
			t.Fatalf("zero start: %v", timer.Value)
		}
		if timer.SourceTick != 80 {
			t.Fatalf("start source tick: %d", timer.SourceTick)
		}
		if timer.FreshnessTicks != 20 {
			t.Fatalf("start freshness: %d", timer.FreshnessTicks)
		}
	}
	for _, timer := range []RunbackFloat{hero.Items[0].ChargeRechargeStart, hero.Abilities[0].ChargeRechargeStart} {
		if timer.Value != 9.25 {
			t.Fatalf("charge start: %v", timer.Value)
		}
		if timer.SourceTick != 82 {
			t.Fatalf("charge source tick: %d", timer.SourceTick)
		}
	}

	// Invalid and absent endpoints remain different missing evidence.
	if hero.Items[0].ChargeRechargeEnd.Present {
		t.Fatal("nonfinite charge end became present")
	}
	if hero.Items[0].ChargeRechargeEnd.MissingReason != RunbackMissingNonFinite {
		t.Fatal("nonfinite reason lost")
	}
	if hero.Abilities[0].ChargeRechargeEnd.Present {
		t.Fatal("absent charge end became present")
	}
	if hero.Abilities[0].ChargeRechargeEnd.MissingReason != RunbackMissingNotInSample {
		t.Fatal("absence reason lost")
	}
}
