package s2replay

import (
	"github.com/paralin/s2replay/protocol"
	"testing"
)

func TestNetworkClockPreservesObservedTickWithoutChangingDemoTime(t *testing.T) {
	p := &Parser{clock: newClock()}
	networkTick := uint32(1798)
	if err := p.applyDecodedMessage(^uint32(0), &protocol.CNETMsg_Tick{Tick: &networkTick}); err != nil {
		t.Fatal(err)
	}
	if _, _, known := p.Clock().ServerTick(); known {
		t.Fatal("signon became a gameplay clock")
	}
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
	networkTick = 0
	if err := p.applyDecodedMessage(101, &protocol.CNETMsg_Tick{Tick: &networkTick}); err != nil {
		t.Fatal(err)
	}
	tick, source, known = p.Clock().ServerTick()
	if !known || tick != 0 || source != 101 {
		t.Fatal("explicit zero network tick lost")
	}
}
