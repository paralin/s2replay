package s2replay

import (
	"context"
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
// container.
type Parser struct {
	// ctx bounds this parser's lifetime when constructed with a context.
	// Cancellation is observed between outer commands; Parser remains single-threaded.
	ctx context.Context
	// r is the underlying byte reader over the demo stream.
	r reader
	// clock tracks the parser's current tick and derived game time.
	clock *Clock
	// serverMap is the map name reported by ServerInfo.
	serverMap string
	// serverGame is the game directory reported by ServerInfo.
	serverGame string
	// rootWorlds maps spawn group handles to their world names.
	rootWorlds map[uint32]string
	// lookahead holds a command read past the requested boundary.
	lookahead *Command
	// pending is the queue of decoded messages awaiting consumption.
	pending []*Message
	// pendingSamples is the queue of entity samples awaiting consumption.
	pendingSamples []EntitySample
	// pendingModifiers is the queue of modifier events awaiting consumption.
	pendingModifiers []ModifierEvent
	// pendingEvents is the queue of unified events awaiting consumption.
	pendingEvents []Event
	// chargeLastSeen remembers the last charge count per ability entity.
	chargeLastSeen map[entityEpoch]int32
	// jumpLastSeen retains observed jump state for each entity generation.
	jumpLastSeen map[entityEpoch]jumpState
	// stopped makes the next Next call report io.EOF once set.
	stopped bool
	// eventOnly suppresses sample retention when set.
	eventOnly bool
	// worldSnapshotMode suppresses event retention during snapshot walks.
	worldSnapshotMode bool

	// classIDBits is the bit width used to encode entity class ids.
	classIDBits uint8
	// classesByID maps class ids to their class records.
	classesByID map[int32]*entityClass
	// classesByName maps class names to their class records.
	classesByName map[string]*entityClass
	// classBaselines maps class ids to their serialized baseline state.
	classBaselines map[int32][]byte
	// serializers maps serializer names to their field layout.
	serializers map[string]*serializer
	// entities maps entity indices to their current state.
	entities map[int32]*Entity
	// modifiers maps modifier indices to their current state.
	modifiers map[int32]modifierState
	// playerItems maps player slots to their owned item ids.
	playerItems map[int32]map[uint32]struct{}
	// entityPlayerSlots maps entity indices to player slots.
	entityPlayerSlots map[int32]int32
	// stringTables holds the decoded string tables.
	stringTables *stringTables
	// entityStateErrors counts entity decode failures by message.
	entityStateErrors map[string]int
	// skippedMessages counts packet or user messages skipped by kind.
	skippedMessages map[skippedMessageKey]int
	// lastControllerSample remembers the last sampled tick per controller.
	lastControllerSample map[int32]uint32
	// firstEntityError records the first entity decode failure message.
	firstEntityError string
	// seenFullPacket indicates a full packet has been processed.
	seenFullPacket bool
	// applyingFullPacket indicates a full packet payload is being unpacked.
	applyingFullPacket bool
	// entityCreates counts entity create operations.
	entityCreates int
	// entityUpdates counts entity update operations.
	entityUpdates int
	// entityDeletes counts entity delete operations.
	entityDeletes int
	// entityLeaves counts entity leave operations.
	entityLeaves int
}

// NewParser validates the PBDEMS2 header and returns a Parser positioned at the
// first command. The demo slice is retained; command payloads alias it.
func NewParser(demo []byte) (*Parser, error) {
	if len(demo) < demoHeaderSize || string(demo[:len(demoMagic)]) != demoMagic {
		return nil, errBadMagic
	}
	return &Parser{
		r:                    reader{buf: demo[demoHeaderSize:]},
		clock:                newClock(),
		classesByID:          make(map[int32]*entityClass),
		classesByName:        make(map[string]*entityClass),
		classBaselines:       make(map[int32][]byte),
		serializers:          make(map[string]*serializer),
		entities:             make(map[int32]*Entity),
		modifiers:            make(map[int32]modifierState),
		playerItems:          make(map[int32]map[uint32]struct{}),
		entityPlayerSlots:    make(map[int32]int32),
		chargeLastSeen:       make(map[entityEpoch]int32),
		jumpLastSeen:         make(map[entityEpoch]jumpState),
		stringTables:         newStringTables(),
		entityStateErrors:    make(map[string]int),
		skippedMessages:      make(map[skippedMessageKey]int),
		lastControllerSample: make(map[int32]uint32),
	}, nil
}

// NewParserWithContext constructs a parser whose command reads stop on context
// cancellation. It retains ctx and demo for its lifetime. A command already
// being decoded finishes before the next read observes cancellation.
func NewParserWithContext(ctx context.Context, demo []byte) (*Parser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := NewParser(demo)
	if err != nil {
		return nil, err
	}
	p.ctx = ctx
	return p, nil
}

// Clock returns the game-time clock advanced by Next.
func (p *Parser) Clock() *Clock { return p.clock }

// Stop makes the next Next call report io.EOF.
func (p *Parser) Stop() { p.stopped = true }

// Next reads the next outer command, decompressing its payload when the
// compression bit is set, and advances the clock. It returns io.EOF once the
// stream is exhausted or after Stop. A context-bound parser returns its context
// error before reading another command when canceled.
func (p *Parser) Next() (*Command, error) {
	// Cancellation bounds every consumer that advances this command stream.
	if p.ctx != nil {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
	}

	// Honor explicit stops and the command retained by a bounded snapshot walk.
	if p.stopped {
		return nil, io.EOF
	}
	if p.lookahead != nil {
		command := p.lookahead
		p.lookahead = nil
		if command.Tick != PreGameTick {
			p.clock.setTick(command.Tick)
		}
		return command, nil
	}
	if p.r.remaining() == 0 {
		return nil, io.EOF
	}

	// Decode the command envelope before interpreting its payload.
	rawKind, err := p.r.readUvarint()
	if err != nil {
		return nil, err
	}
	kind := int32(rawKind)
	compressed := kind&demoIsCompressed != 0
	kind &^= demoIsCompressed

	// Read the selected tick and its bounded command payload.
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

	// Only gameplay commands advance the visible clock.
	t := uint32(tick)
	if t != PreGameTick {
		p.clock.setTick(t)
	}
	return &Command{Kind: protocol.EDemoCommands(kind), Tick: t, Payload: payload}, nil
}

// ServerWorld identifies the active root world at the current parser tick.
// A loaded root world supersedes the server's bootstrap map. Multiple distinct
// non-bootstrap roots are ambiguous and return an empty map name.
func (p *Parser) ServerWorld() (game, mapName string) {
	mapName = p.serverMap
	for _, world := range p.rootWorlds {
		if world == p.serverMap {
			continue
		}
		if mapName != p.serverMap && mapName != world {
			return p.serverGame, ""
		}
		mapName = world
	}
	return p.serverGame, mapName
}
