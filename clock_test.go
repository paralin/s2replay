package s2replay

import (
	"testing"

	"github.com/paralin/s2replay/protocol"
)

// TestNetworkClockPreservesObservedTickWithoutChangingDemoTime verifies clock separation.
func TestNetworkClockPreservesObservedTickWithoutChangingDemoTime(t *testing.T) {
	// Start with a parser whose demo clock is at its zero value.
	p := &Parser{clock: newClock()}
	networkTick := uint32(1798)

	// Ignore signon network ticks when reporting the gameplay clock.
	if err := p.applyDecodedMessage(^uint32(0), &protocol.CNETMsg_Tick{Tick: &networkTick}); err != nil {
		t.Fatal(err)
	}
	if _, _, known := p.Clock().ServerTick(); known {
		t.Fatal("signon became a gameplay clock")
	}

	// Record gameplay network ticks independently from the demo clock.
	p.clock.setTick(100)
	networkTick = 1898
	if err := p.applyDecodedMessage(99, &protocol.CNETMsg_Tick{Tick: &networkTick}); err != nil {
		t.Fatal(err)
	}
	if err := p.applyDecodedMessage(100, &protocol.CNETMsg_Tick{}); err != nil {
		t.Fatal(err)
	}
	tick, source, known := p.Clock().ServerTick()
	if !known || tick != 1898 || source != 99 {
		t.Fatalf("network tick=%d source=%d known=%v", tick, source, known)
	}
	if p.Clock().Tick() != 100 || p.Clock().GameTime() != 100*DefaultTickInterval {
		t.Fatal("network tick changed demo clock")
	}

	// Preserve an explicitly observed zero network tick.
	networkTick = 0
	if err := p.applyDecodedMessage(101, &protocol.CNETMsg_Tick{Tick: &networkTick}); err != nil {
		t.Fatal(err)
	}
	tick, source, known = p.Clock().ServerTick()
	if !known || tick != 0 || source != 101 {
		t.Fatal("explicit zero network tick lost")
	}
}
