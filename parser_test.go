package s2replay

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

// buildDemo assembles a minimal valid PBDEMS2 container from outer records.
func buildDemo(t *testing.T, recs []Command) []byte {
	t.Helper()
	buf := append([]byte(demoMagic), make([]byte, demoHeaderSize-len(demoMagic))...)
	for _, rec := range recs {
		buf = binary.AppendUvarint(buf, uint64(rec.Kind))
		buf = binary.AppendUvarint(buf, uint64(rec.Tick))
		buf = binary.AppendUvarint(buf, uint64(len(rec.Payload)))
		buf = append(buf, rec.Payload...)
	}
	return buf
}

func TestNewParserRejectsBadMagic(t *testing.T) {
	if _, err := NewParser([]byte("not a demo at all!!")); err != errBadMagic {
		t.Fatalf("want errBadMagic, got %v", err)
	}
	if _, err := NewParser([]byte("PB")); err != errBadMagic {
		t.Fatalf("short input: want errBadMagic, got %v", err)
	}
}

func TestParserWalksCommandsAndClock(t *testing.T) {
	header := &protocol.CDemoFileHeader{DemoFileStamp: proto(demoMagic), MapName: proto("dl_midtown")}
	headerBytes, err := header.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	demo := buildDemo(t, []Command{
		{Kind: protocol.EDemoCommands_DEM_FileHeader, Tick: PreGameTick, Payload: headerBytes},
		{Kind: protocol.EDemoCommands_DEM_Packet, Tick: 64, Payload: []byte{0x01}},
		{Kind: protocol.EDemoCommands_DEM_Packet, Tick: 128, Payload: []byte{0x02}},
		{Kind: protocol.EDemoCommands_DEM_Stop, Tick: 128},
	})

	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}

	first, err := p.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != protocol.EDemoCommands_DEM_FileHeader {
		t.Fatalf("first kind: want DEM_FileHeader, got %s", first.Kind)
	}
	got := &protocol.CDemoFileHeader{}
	if err := got.UnmarshalVT(first.Payload); err != nil {
		t.Fatal(err)
	}
	if got.GetMapName() != "dl_midtown" {
		t.Fatalf("map name: want dl_midtown, got %q", got.GetMapName())
	}
	// The pre-game sentinel tick must not advance the clock.
	if p.Clock().Tick() != 0 {
		t.Fatalf("clock advanced on sentinel tick: %d", p.Clock().Tick())
	}

	if _, err := p.Next(); err != nil {
		t.Fatal(err)
	}
	if p.Clock().Tick() != 64 {
		t.Fatalf("tick: want 64, got %d", p.Clock().Tick())
	}
	if want := 64.0 * DefaultTickInterval; p.Clock().GameTime() != want {
		t.Fatalf("game time: want %v, got %v", want, p.Clock().GameTime())
	}

	for {
		if _, err := p.Next(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Next(); err != io.EOF {
		t.Fatalf("after exhaustion: want io.EOF, got %v", err)
	}
}

func proto(s string) *string { return &s }
