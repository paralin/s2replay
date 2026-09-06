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

// TestNewParserRejectsBadMagic verifies that malformed demo headers are rejected.
func TestNewParserRejectsBadMagic(t *testing.T) {
	if _, err := NewParser([]byte("not a demo at all!!")); err != errBadMagic {
		t.Fatalf("want errBadMagic, got %v", err)
	}
	if _, err := NewParser([]byte("PB")); err != errBadMagic {
		t.Fatalf("short input: want errBadMagic, got %v", err)
	}
}

// TestParserWalksCommandsAndClock verifies command iteration and demo clock updates.
func TestParserWalksCommandsAndClock(t *testing.T) {
	// Build a demo containing a header, packet ticks, and a stop command.
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

	// Read the header command and verify its decoded metadata.
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
	// Confirm that the pre-game sentinel tick does not advance the clock.
	if p.Clock().Tick() != 0 {
		t.Fatalf("clock advanced on sentinel tick: %d", p.Clock().Tick())
	}

	if _, err := p.Next(); err != nil {
		t.Fatal(err)
	}

	// Confirm that the first gameplay tick advances the demo clock.
	if p.Clock().Tick() != 64 {
		t.Fatalf("tick: want 64, got %d", p.Clock().Tick())
	}
	if want := 64.0 * DefaultTickInterval; p.Clock().GameTime() != want {
		t.Fatalf("game time: want %v, got %v", want, p.Clock().GameTime())
	}

	// Consume the remaining commands and verify stable exhaustion behavior.
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

// proto returns a pointer to a string literal value.
func proto(s string) *string { return &s }

// TestServerWorldTracksDecodedServerInfo verifies server identity replacement.
func TestServerWorldTracksDecodedServerInfo(t *testing.T) {
	p, err := NewParser(buildDemo(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if game, mapName := p.ServerWorld(); game != "" || mapName != "" {
		t.Fatal("unobserved world must stay unknown")
	}
	p.applyServerInfo(&protocol.CSVCMsg_ServerInfo{GameDir: proto("citadel"), MapName: proto("dl_midtown")})
	if game, mapName := p.ServerWorld(); game != "citadel" || mapName != "dl_midtown" {
		t.Fatalf("server world: %q %q", game, mapName)
	}
	p.applyServerInfo(&protocol.CSVCMsg_ServerInfo{MapName: proto("training")})
	if game, mapName := p.ServerWorld(); game != "" || mapName != "training" {
		t.Fatalf("must not retain a previous world's identity: %q %q", game, mapName)
	}
}

// TestRecoveryWorldIdentityAndUnload verifies recovered world identity tracking.
func TestRecoveryWorldIdentityAndUnload(t *testing.T) {
	p, err := NewParser(buildDemo(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	p.applyServerInfo(&protocol.CSVCMsg_ServerInfo{MapName: proto("start")})
	two, three := uint32(2), uint32(3)
	load := &protocol.CNETMsg_SpawnGroup_Load{Worldname: proto("dl_midtown"), Spawngrouphandle: &two}
	payload, err := load.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	// Encode packet framing as a six-bit message ID, byte varint size, and protobuf.
	var bits []bool
	put := func(value uint64, count int) {
		for i := range count {
			bits = append(bits, value&(1<<i) != 0)
		}
	}
	put(uint64(protocol.NET_Messages_net_SpawnGroup_Load), 6)
	for _, b := range binary.AppendUvarint(nil, uint64(len(payload))) {
		put(uint64(b), 8)
	}
	for _, b := range payload {
		put(uint64(b), 8)
	}
	packet := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b {
			packet[i/8] |= 1 << (i % 8)
		}
	}
	recovery := &protocol.CDemoRecovery{SpawnGroupMessage: packet}
	data, err := recovery.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.queueCommandMessages(&Command{Kind: protocol.EDemoCommands_DEM_Recovery, Tick: PreGameTick, Payload: data}); err != nil {
		t.Fatal(err)
	}
	if _, world := p.ServerWorld(); world != "dl_midtown" {
		t.Fatalf("loaded world = %q", world)
	}
	if err := p.applyDecodedMessage(1, &protocol.CNETMsg_SpawnGroup_Load{Worldname: proto("another"), Spawngrouphandle: &three}); err != nil {
		t.Fatal(err)
	}
	if _, world := p.ServerWorld(); world != "" {
		t.Fatalf("ambiguous world = %q", world)
	}
	if err := p.applyDecodedMessage(2, &protocol.CNETMsg_SpawnGroup_Unload{Spawngrouphandle: &three}); err != nil {
		t.Fatal(err)
	}
	if _, world := p.ServerWorld(); world != "dl_midtown" {
		t.Fatalf("remaining world = %q", world)
	}
}
