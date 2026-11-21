// Command demomoments prints, as JSON lines, the events of each demo that
// mark a moment worth clipping: chat messages, hero kills, kill streaks,
// respawns and shots. It reads only the outer commands and the inner
// message headers, decoding just the messages it reports, so it runs at
// disk speed. It is an experiment for mining clips from demos.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/snappy"
	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

type event struct {
	Demo     string `json:"demo"`
	Tick     uint32 `json:"tick"`
	Kind     string `json:"kind"`
	Slot     int32  `json:"slot,omitempty"`
	Text     string `json:"text,omitempty"`
	AllChat  bool   `json:"all_chat,omitempty"`
	Victim   int32  `json:"victim,omitempty"`
	Attacker int32  `json:"attacker,omitempty"`
	Team     int32  `json:"team,omitempty"`
	Kills    int32  `json:"kills,omitempty"`
	Shooter  int32  `json:"shooter,omitempty"`
	// Parrier is the hero entity whose parry caught a swing; Near is the
	// nearest hero of another team at that moment and Dist its distance.
	Parrier int32   `json:"parrier,omitempty"`
	Near    int32   `json:"near,omitempty"`
	Dist    float32 `json:"dist,omitempty"`
}

func main() {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)
	for _, path := range os.Args[1:] {
		if err := scan(path, enc); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		}
		if err := scanParries(path, enc); err != nil {
			fmt.Fprintf(os.Stderr, "%s: parries: %v\n", path, err)
		}
	}
}

// scan walks one demo and encodes each reported event.
func scan(path string, enc *json.Encoder) error {
	buf, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(buf) < 16 || string(buf[:8]) != "PBDEMS2\x00" {
		return fmt.Errorf("not a PBDEMS2 demo")
	}
	demo := filepath.Base(path)
	var last uint32
	maxTick := uint32(0)
	for pos := 16; pos < len(buf); {
		raw, n := binary.Uvarint(buf[pos:])
		if n <= 0 {
			break
		}
		pos += n
		tick, n := binary.Uvarint(buf[pos:])
		pos += n
		size, n := binary.Uvarint(buf[pos:])
		pos += n
		if pos+int(size) > len(buf) {
			return fmt.Errorf("truncated at %d", pos)
		}
		payload := buf[pos : pos+int(size)]
		pos += int(size)
		kind := protocol.EDemoCommands(raw) &^ protocol.EDemoCommands_DEM_IsCompressed
		if kind != protocol.EDemoCommands_DEM_Packet || uint32(tick) == ^uint32(0) {
			continue
		}
		if raw&uint64(protocol.EDemoCommands_DEM_IsCompressed) != 0 {
			if payload, err = snappy.Decode(nil, payload); err != nil {
				return err
			}
		}
		var pkt protocol.CDemoPacket
		if err := pkt.UnmarshalVT(payload); err != nil {
			return err
		}
		last = uint32(tick)
		maxTick = max(maxTick, last)
		walk(pkt.GetData(), func(k uint32, msg []byte) {
			e := event{Demo: demo, Tick: last}
			switch k {
			case uint32(protocol.CitadelUserMessageIds_k_EUserMsg_ChatMsg):
				var m protocol.CCitadelUserMsg_ChatMsg
				if m.UnmarshalVT(msg) != nil {
					return
				}
				e.Kind, e.Slot, e.Text, e.AllChat = "chat", m.GetPlayerSlot(), m.GetText(), m.GetAllChat()
			case uint32(protocol.CitadelUserMessageIds_k_EUserMsg_HeroKilled):
				var m protocol.CCitadelUserMsg_HeroKilled
				if m.UnmarshalVT(msg) != nil {
					return
				}
				e.Kind, e.Victim, e.Attacker, e.Team = "kill", m.GetEntindexVictim(), m.GetEntindexAttacker(), m.GetVictimTeamNumber()
			case uint32(protocol.CitadelUserMessageIds_k_EUserMsg_KillStreak):
				var m protocol.CCitadelUserMsg_KillStreak
				if m.UnmarshalVT(msg) != nil {
					return
				}
				e.Kind, e.Shooter, e.Kills = "streak", int32(m.GetPlayerPawn()), m.GetNumKills()
			case uint32(protocol.CitadelUserMessageIds_k_EUserMsg_PlayerRespawned):
				e.Kind = "respawn"
			case uint32(protocol.ECitadelGameEvents_GE_FireBullets):
				var m protocol.CMsgFireBullets
				if m.UnmarshalVT(msg) != nil {
					return
				}
				e.Kind, e.Shooter = "shot", m.GetShooterEntity()
			default:
				return
			}
			_ = enc.Encode(e)
		})
	}
	return enc.Encode(event{Demo: demo, Tick: maxTick, Kind: "end"})
}

// hero is the last sampled position and team of a hero entity.
type hero struct {
	pos  [3]float32
	team int32
}

// scanParries walks a demo's entities and reports each parry that caught a
// swing. A parry ability entity is learned from its first press (a new
// cooldown start, s2replay's ability_cast event); a catch is a forward move of
// its m_flParrySuccessEndTime. The swing's attacker is not networked, so the
// event names the nearest hero of another team instead.
func scanParries(path string, enc *json.Encoder) error {
	buf, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(buf)
	if err != nil {
		return err
	}
	demo := filepath.Base(path)
	parry := map[int32]float32{}
	heroes := map[int32]hero{}
	for {
		ev, err := p.NextEvent()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch ev.Type {
		case s2replay.EventAbilityCast:
			if _, ok := parry[ev.Entity]; !ok && strings.Contains(ev.AbilityCast.ClassName, "MeleeParry") {
				if e := p.FindEntity(ev.Entity); e != nil {
					parry[ev.Entity], _ = e.Float32("m_flParrySuccessEndTime")
				}
			}
		case s2replay.EventEntitySample:
			s := ev.EntitySample
			if strings.HasPrefix(s.ClassName, "CCitadelPlayerPawn") {
				heroes[ev.Entity] = hero{pos: [3]float32{s.PositionX, s.PositionY, s.PositionZ}, team: s.Team}
			}
		}
		for idx, last := range parry {
			e := p.FindEntity(idx)
			if e == nil {
				continue
			}
			end, ok := e.Float32("m_flParrySuccessEndTime")
			if !ok || end <= last {
				continue
			}
			parry[idx] = end
			out := event{Demo: demo, Tick: ev.Tick, Kind: "parry", Near: -1}
			if h, ok := e.UInt32("m_hOwnerEntity"); ok {
				if owner := p.FindEntityByHandle(uint64(h)); owner != nil {
					out.Parrier = owner.Index()
				}
			}
			me := heroes[out.Parrier]
			best := float32(math.MaxFloat32)
			for i, h := range heroes {
				if i == out.Parrier || h.team == me.team {
					continue
				}
				dx, dy, dz := h.pos[0]-me.pos[0], h.pos[1]-me.pos[1], h.pos[2]-me.pos[2]
				if d := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz))); d < best {
					best, out.Near, out.Dist = d, i, d
				}
			}
			_ = enc.Encode(out)
		}
	}
}

// walk calls fn with the kind and bytes of each inner message of a packet.
func walk(data []byte, fn func(kind uint32, msg []byte)) {
	r := &bitReader{buf: data}
	for r.remaining() > 8 {
		k := r.ubitvar()
		sz := int(r.varint())
		if sz*8 > r.remaining() {
			return
		}
		if r.pos%8 == 0 {
			fn(k, data[r.pos/8:r.pos/8+sz])
		} else {
			msg := make([]byte, sz)
			for i := range msg {
				msg[i] = byte(r.bits(8))
			}
			r.pos -= sz * 8
			fn(k, msg)
		}
		r.pos += sz * 8
	}
}

type bitReader struct {
	buf []byte
	pos int
}

func (r *bitReader) remaining() int { return len(r.buf)*8 - r.pos }

func (r *bitReader) bits(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		v |= uint32(r.buf[r.pos>>3]>>(r.pos&7)&1) << i
		r.pos++
	}
	return v
}

func (r *bitReader) ubitvar() uint32 {
	v := r.bits(6)
	switch v & 0x30 {
	case 0x10:
		v = v&15 | r.bits(4)<<4
	case 0x20:
		v = v&15 | r.bits(8)<<4
	case 0x30:
		v = v&15 | r.bits(28)<<4
	}
	return v
}

func (r *bitReader) varint() uint32 {
	var v uint32
	for s := 0; s < 35; s += 7 {
		b := r.bits(8)
		v |= (b & 0x7f) << s
		if b&0x80 == 0 {
			break
		}
	}
	return v
}
