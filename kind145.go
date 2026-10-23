package s2replay

import (
	"encoding/binary"
	"math"
)

// kind145 is the inner demo packet message kind carrying Deadlock's
// per-tick game state envelope.
const kind145 int32 = 145

// Inner demo packet message kind 145 carries Deadlock's per-tick game state
// as a protobuf envelope: field 1 is a subtype, field 2 an entity index, and
// subtype 27 embeds a transform message (field 1 update type, fields 2 and 3
// nested vec3 messages with fixed32 float fields). Kind-55 packet entities
// only carry sparse keyframes mid-match; the dense pawn motion rides here.

type pbField struct {
	num    int
	wire   int
	varint uint64
	fixed  []byte
	data   []byte
}

// walkRawPB walks one protobuf message without a schema.
func walkRawPB(buf []byte) ([]pbField, error) {
	out := make([]pbField, 0, 8)
	pos := 0
	for pos < len(buf) {
		key, n := binary.Uvarint(buf[pos:])
		if n <= 0 {
			return out, errUnknownFieldPath
		}
		pos += n
		f := pbField{num: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(buf[pos:])
			if n <= 0 {
				return out, errUnknownFieldPath
			}
			f.varint = v
			pos += n
		case 1:
			if pos+8 > len(buf) {
				return out, errUnknownFieldPath
			}
			f.fixed = buf[pos : pos+8]
			pos += 8
		case 2:
			l, n := binary.Uvarint(buf[pos:])
			if n <= 0 || pos+n+int(l) > len(buf) {
				return out, errUnknownFieldPath
			}
			pos += n
			f.data = buf[pos : pos+int(l)]
			pos += int(l)
		case 5:
			if pos+4 > len(buf) {
				return out, errUnknownFieldPath
			}
			f.fixed = buf[pos : pos+4]
			pos += 4
		default:
			return out, errUnknownFieldPath
		}
		out = append(out, f)
	}
	return out, nil
}

func pbFnum(fs []pbField, num int) (pbField, bool) {
	for _, f := range fs {
		if f.num == num {
			return f, true
		}
	}
	return pbField{}, false
}

// pbVec3 reads a nested vec3 message: fields 1-3 as fixed32 floats.
func pbVec3(f pbField) ([3]float32, bool) {
	var out [3]float32
	fs, err := walkRawPB(f.data)
	if err != nil {
		return out, false
	}
	got := 0
	for _, ff := range fs {
		if ff.num >= 1 && ff.num <= 3 && ff.wire == 5 && len(ff.fixed) == 4 {
			out[ff.num-1] = math.Float32frombits(binary.LittleEndian.Uint32(ff.fixed))
			got++
		}
	}
	return out, got == 3
}

// applyKind145 decodes one kind-145 envelope and appends position-bearing
// entity samples for hero-class pawns. Malformed payloads are dropped:
// this stream is supplementary evidence on top of the keyframe path.
func (p *Parser) applyKind145(tick uint32, buf []byte) {
	outer, err := walkRawPB(buf)
	if err != nil {
		return
	}
	sub, ok := pbFnum(outer, 1)
	if !ok || sub.varint != 27 {
		return
	}
	eidF, ok := pbFnum(outer, 2)
	if !ok {
		return
	}
	tf, ok := pbFnum(outer, 30)
	if !ok || tf.wire != 2 {
		return
	}
	inner, err := walkRawPB(tf.data)
	if err != nil {
		return
	}
	originF, ok := pbFnum(inner, 2)
	if !ok || originF.wire != 2 {
		return
	}
	origin, ok := pbVec3(originF)
	if !ok {
		return
	}
	e := p.FindEntity(int32(eidF.varint))
	if e == nil {
		return
	}
	p.appendKind145Sample(tick, e, origin)
}

// appendKind145Sample appends a position-only sample for a hero-class pawn,
// keeping the entity's slot attribution from the keyframe path.
func (p *Parser) appendKind145Sample(tick uint32, e *Entity, origin [3]float32) {
	if e.class == nil || !e.active || !isLikelyHeroClass(e.class.name) {
		return
	}
	sample := EntitySample{
		Tick:        tick,
		GameTime:    p.clock.GameTime(),
		Entity:      e.index,
		ClassID:     e.class.id,
		ClassName:   e.class.name,
		PositionX:   origin[0],
		PositionY:   origin[1],
		PositionZ:   origin[2],
		HasPosition: true,
	}
	if !p.eventOnly {
		p.pendingSamples = append(p.pendingSamples, sample)
	}
	slot, ok := p.entityPlayerSlots[sample.Entity]
	if !ok {
		slot = -1
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:         EventEntitySample,
		Tick:         sample.Tick,
		GameTime:     sample.GameTime,
		Entity:       sample.Entity,
		PlayerSlot:   slot,
		EntitySample: &sample,
	})
}
