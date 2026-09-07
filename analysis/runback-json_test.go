package analysis

import (
	"bytes"
	"reflect"
	"testing"
)

func TestRunbackFactsJSONRoundTrip(t *testing.T) {
	// Exercise typed evidence, missing values, fixed arrays and optional slices.
	facts, err := buildRunbackFacts(nil, Result{}, ReplaySourceIdentity{SHA256: "source"}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	facts.EligibilityReasons = nil
	facts.Heroes = []RunbackHero{{
		PlayerSlot: 3,
		ClassName:  "pawn\"<hero>",
		HeroID:     RunbackUint{Value: ^uint32(0)},
	}}
	encoded, err := facts.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"value":4294967295`)) {
		t.Fatal("codec lost the full-width unsigned evidence value")
	}
	if bytes.Contains(encoded, []byte(`"eligibility_reasons"`)) {
		t.Fatal("codec emitted absent optional eligibility reasons")
	}

	// The owner decoder must preserve the existing replay-facts representation.
	var decoded RunbackFacts
	if err := decoded.UnmarshalJSON(encoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(facts, decoded) {
		t.Fatal("codec changed replay evidence during round trip")
	}
	if err := decoded.UnmarshalJSON([]byte(`{"tick":`)); err == nil {
		t.Fatal("codec accepted truncated replay facts")
	}
}
