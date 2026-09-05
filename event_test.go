package s2replay

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSanitizeEventRemovesNonFiniteFloats(t *testing.T) {
	ev := Event{
		GameTime: math.NaN(),
		Damage: &DamageEvent{
			GameTime:         math.Inf(1),
			PreDamage:        float32(math.NaN()),
			DamageAbsorbed:   float32(math.Inf(1)),
			Effectiveness:    float32(math.NaN()),
			CritDamage:       float32(math.Inf(-1)),
			OriginX:          float32(math.NaN()),
			DamageDirectionZ: float32(math.Inf(1)),
		},
		Modifier: &ModifierEvent{
			HasLastAppliedTime: true,
			HasDuration:        true,
			GameTime:           math.NaN(),
			LastAppliedTime:    float32(math.Inf(1)),
			Duration:           float32(math.NaN()),
		},
		Purchase: &PurchaseEvent{GameTime: math.Inf(-1)},
		EntitySample: &EntitySample{
			GameTime:    math.NaN(),
			Health:      float32(math.NaN()),
			MaxHealth:   100,
			Shield:      10,
			MaxShield:   float32(math.Inf(1)),
			PositionX:   1,
			PositionY:   float32(math.NaN()),
			PositionZ:   3,
			HasHealth:   true,
			HasShield:   true,
			HasPosition: true,
		},
	}

	sanitizeEvent(&ev)

	if ev.GameTime != 0 || ev.Damage.GameTime != 0 || ev.Modifier.GameTime != 0 || ev.Purchase.GameTime != 0 {
		t.Fatalf("non-finite game time was not sanitized: %+v", ev)
	}
	if ev.Damage.PreDamage != 0 || ev.Damage.DamageAbsorbed != 0 || ev.Damage.Effectiveness != 0 || ev.Damage.CritDamage != 0 {
		t.Fatalf("non-finite damage floats were not sanitized: %+v", ev.Damage)
	}
	if ev.Modifier.LastAppliedTime != 0 || ev.Modifier.Duration != 0 || ev.Modifier.HasLastAppliedTime || ev.Modifier.HasDuration {
		t.Fatalf("non-finite modifier floats were not sanitized: %+v", ev.Modifier)
	}
	if ev.EntitySample.HasHealth || ev.EntitySample.HasShield || ev.EntitySample.HasPosition {
		t.Fatalf("non-finite entity sample flags were not cleared: %+v", ev.EntitySample)
	}
	if _, err := json.Marshal(ev); err != nil {
		t.Fatalf("sanitized event is not JSON encodable: %v", err)
	}
}
