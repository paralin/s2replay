package s2replay

import (
	"io"

	"github.com/klauspost/compress/snappy"

	"github.com/paralin/s2replay/protocol"
)

// demoMagic is the PBDEMS2 file signature, including its trailing NUL.
const demoMagic = "PBDEMS2\x00"

// demoHeaderSize is the fixed prefix skipped before the command stream: the
// 8-byte magic plus 8 reserved bytes.
const demoHeaderSize = 16

// demoIsCompressed is the EDemoCommands bit marking a snappy-compressed payload.
const demoIsCompressed = int32(protocol.EDemoCommands_DEM_IsCompressed)

// PreGameTick is the sentinel tick Source 2 stamps on pre-game commands; it is
// ignored for clock advancement so game time stays monotonic.
const PreGameTick = ^uint32(0)

// Command is one outer demo record: its kind, the tick it applies to, and the
// decompressed payload bytes awaiting message decode.
type Command struct {
	Kind    protocol.EDemoCommands
	Tick    uint32
	Payload []byte
}

// Parser walks a Source 2 PBDEMS2 demo container. It validates the header,
// yields the outer command stream one record at a time, and owns the Clock.
// Packet unpacking, message dispatch, and entity decoding layer on top of this
// container in later phases.
type Parser struct {
	r       reader
	clock   *Clock
	stopped bool
}

// NewParser validates the PBDEMS2 header and returns a Parser positioned at the
// first command. The demo slice is retained; command payloads alias it.
func NewParser(demo []byte) (*Parser, error) {
	if len(demo) < demoHeaderSize || string(demo[:len(demoMagic)]) != demoMagic {
		return nil, errBadMagic
	}
	return &Parser{r: reader{buf: demo[demoHeaderSize:]}, clock: newClock()}, nil
}

// Clock returns the game-time clock advanced by Next.
func (p *Parser) Clock() *Clock { return p.clock }

// Stop makes the next Next call report io.EOF.
func (p *Parser) Stop() { p.stopped = true }

// Next reads the next outer command, decompressing its payload when the
// compression bit is set, and advances the clock. It returns io.EOF once the
// stream is exhausted or after Stop.
func (p *Parser) Next() (*Command, error) {
	if p.stopped || p.r.remaining() == 0 {
		return nil, io.EOF
	}

	rawKind, err := p.r.readUvarint()
	if err != nil {
		return nil, err
	}
	kind := int32(rawKind)
	compressed := kind&demoIsCompressed != 0
	kind &^= demoIsCompressed

	tick, err := p.r.readUvarint()
	if err != nil {
		return nil, err
	}
	size, err := p.r.readUvarint()
	if err != nil {
		return nil, err
	}
	payload, err := p.r.readBytes(int(size))
	if err != nil {
		return nil, err
	}
	if compressed {
		payload, err = snappy.Decode(nil, payload)
		if err != nil {
			return nil, err
		}
	}

	t := uint32(tick)
	if t != PreGameTick {
		p.clock.setTick(t)
	}
	return &Command{Kind: protocol.EDemoCommands(kind), Tick: t, Payload: payload}, nil
}
