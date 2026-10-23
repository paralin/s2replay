package s2replay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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
	// src is the outer command stream, read one command at a time.
	src *bufio.Reader
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
	// castLastSeen remembers the last cooldown start per ability entity.
	castLastSeen map[entityEpoch]float32
	// trooperLastSample remembers the tick of each trooper's last sample.
	trooperLastSample map[entityEpoch]uint32
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
	// maxCoord is the world coordinate bound the server announces in
	// ServerInfo's GameSessionConfiguration. Deadlock moved it from 16384 to
	// 32768 in a later build, which moves the cell origin; world coordinates
	// are reconstructed relative to it.
	maxCoord float32
	// classesByID maps class ids to their class records.
	classesByID map[int32]*entityClass
	// classesByName maps class names to their class records.
	classesByName map[string]*entityClass
	// fieldPaths is the scratch buffer readFields decodes each update's paths into.
	fieldPaths []fieldPath
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
// first command. It reads demo as a stream; see NewParserFromReader.
func NewParser(demo []byte) (*Parser, error) {
	return NewParserFromReader(context.Background(), bytes.NewReader(demo))
}

// NewParserWithContext constructs a parser whose command reads stop on context
// cancellation. It retains ctx for its lifetime. A command already being
// decoded finishes before the next read observes cancellation.
func NewParserWithContext(ctx context.Context, demo []byte) (*Parser, error) {
	return NewParserFromReader(ctx, bytes.NewReader(demo))
}

// NewParserFromReader validates the PBDEMS2 header and returns a Parser that
// reads the command stream from src one command at a time, so memory use does
// not grow with the size of the replay. The parser consumes src until it
// reports io.EOF and does not close it.
func NewParserFromReader(ctx context.Context, src io.Reader) (*Parser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(src, 1<<20)
	header := make([]byte, demoHeaderSize)
	if _, err := io.ReadFull(br, header); err != nil || string(header[:len(demoMagic)]) != demoMagic {
		return nil, errBadMagic
	}
	return &Parser{
		ctx:                  ctx,
		src:                  br,
		clock:                newClock(),
		classesByID:          make(map[int32]*entityClass),
		classesByName:        make(map[string]*entityClass),
		classBaselines:       make(map[int32][]byte),
		maxCoord:             deadlockDefaultMaxCoord,
		serializers:          make(map[string]*serializer),
		entities:             make(map[int32]*Entity),
		modifiers:            make(map[int32]modifierState),
		playerItems:          make(map[int32]map[uint32]struct{}),
		entityPlayerSlots:    make(map[int32]int32),
		chargeLastSeen:       make(map[entityEpoch]int32),
		castLastSeen:         make(map[entityEpoch]float32),
		trooperLastSample:    make(map[entityEpoch]uint32),
		jumpLastSeen:         make(map[entityEpoch]jumpState),
		stringTables:         newStringTables(),
		entityStateErrors:    make(map[string]int),
		skippedMessages:      make(map[skippedMessageKey]int),
		lastControllerSample: make(map[int32]uint32),
	}, nil
}

// readUvarint reads one varint of the command envelope. A stream that ends
// before the first byte reports io.EOF; one that ends inside the varint
// reports a malformed varint.
func (p *Parser) readUvarint() (uint64, error) {
	v, err := binary.ReadUvarint(p.src)
	if err == io.EOF {
		return 0, io.EOF
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, errInvalidVarint
	}
	return v, err
}

// readPayload reads the next size bytes of the command stream into a fresh
// buffer, which the command owns. The buffer grows with the bytes that arrive,
// so a corrupt size cannot reserve memory the stream does not supply.
func (p *Parser) readPayload(size uint64) ([]byte, error) {
	if size <= 1<<20 {
		payload := make([]byte, size)
		if _, err := io.ReadFull(p.src, payload); err != nil {
			return nil, errShortRead
		}
		return payload, nil
	}
	payload, err := io.ReadAll(io.LimitReader(p.src, int64(size)))
	if err != nil {
		return nil, err
	}
	if uint64(len(payload)) != size {
		return nil, errShortRead
	}
	return payload, nil
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
	// A zero Parser has no stream.
	if p.src == nil {
		return nil, io.EOF
	}

	// Decode the command envelope before interpreting its payload.
	rawKind, err := p.readUvarint()
	if err != nil {
		return nil, err
	}
	kind := int32(rawKind)
	compressed := kind&demoIsCompressed != 0
	kind &^= demoIsCompressed

	// Read the selected tick and its bounded command payload.
	tick, err := p.readUvarint()
	if err != nil {
		return nil, err
	}
	size, err := p.readUvarint()
	if err != nil {
		return nil, err
	}
	payload, err := p.readPayload(size)
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
