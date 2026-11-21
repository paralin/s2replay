// Command democlip cuts tick ranges out of PBDEMS2 demos and joins them into
// one demo that the game plays from its first tick.
//
//	democlip -o out.dem src.dem:A-B [src2.dem:C-D ...]
//
// Each segment starts with a synthetic full packet: every string table at its
// latest copy and the entity snapshot of the source's last full packet at or
// before A. The deltas between that snapshot and A follow at the segment's
// first tick, stripped to state messages, so playback applies them in one
// frame. Commands from A to B follow with their ticks moved so the output's
// ticks rise from zero.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/compress/snappy"
	"github.com/paralin/s2replay/protocol"
)

const preGameTick = ^uint32(0)

// command is one outer demo record. raw holds the payload as stored, which
// is snappy-compressed when compressed is set.
type command struct {
	kind       protocol.EDemoCommands
	compressed bool
	tick       uint32
	raw        []byte
}

func (c *command) payload() []byte {
	if !c.compressed {
		return c.raw
	}
	p, err := snappy.Decode(nil, c.raw)
	must(err)
	return p
}

// demo is a parsed container: the signon through DEM_SyncTick, the game
// commands, and the trailer records.
type demo struct {
	signon      []*command
	game        []*command
	spawnGroups *command
	fileInfo    *protocol.CDemoFileInfo
}

func readDemo(path string) *demo {
	buf, err := os.ReadFile(path)
	must(err)
	if string(buf[:8]) != "PBDEMS2\x00" {
		panic(path + ": not a PBDEMS2 demo")
	}
	d := &demo{}
	pos := 16
	inSignon := true
	for pos < len(buf) {
		raw, n := binary.Uvarint(buf[pos:])
		pos += n
		tick, n := binary.Uvarint(buf[pos:])
		pos += n
		size, n := binary.Uvarint(buf[pos:])
		pos += n
		c := &command{
			kind:       protocol.EDemoCommands(int32(raw) &^ int32(protocol.EDemoCommands_DEM_IsCompressed)),
			compressed: int32(raw)&int32(protocol.EDemoCommands_DEM_IsCompressed) != 0,
			tick:       uint32(tick),
			raw:        buf[pos : pos+int(size)],
		}
		pos += int(size)
		switch {
		case inSignon:
			d.signon = append(d.signon, c)
			if c.kind == protocol.EDemoCommands_DEM_SyncTick {
				inSignon = false
			}
		case c.kind == protocol.EDemoCommands_DEM_Stop:
		case c.kind == protocol.EDemoCommands_DEM_SpawnGroups:
			d.spawnGroups = c
		case c.kind == protocol.EDemoCommands_DEM_FileInfo:
			d.fileInfo = &protocol.CDemoFileInfo{}
			must(d.fileInfo.UnmarshalVT(c.payload()))
		default:
			d.game = append(d.game, c)
		}
	}
	return d
}

// fingerprint hashes the signon records a joined demo must share with the
// first segment's source: the send tables and the class list.
func (d *demo) fingerprint() [32]byte {
	h := sha256.New()
	for _, c := range d.signon {
		if c.kind == protocol.EDemoCommands_DEM_SendTables || c.kind == protocol.EDemoCommands_DEM_ClassInfo {
			h.Write(c.payload())
		}
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// tableState holds the latest full copy of each string table, in signon order.
type tableState struct {
	order  []string
	tables map[string]*protocol.CDemoStringTablesTableT
}

func (s *tableState) apply(st *protocol.CDemoStringTables) {
	for _, t := range st.GetTables() {
		if _, ok := s.tables[t.GetTableName()]; !ok {
			s.order = append(s.order, t.GetTableName())
		}
		s.tables[t.GetTableName()] = t
	}
}

func (s *tableState) snapshot() *protocol.CDemoStringTables {
	out := &protocol.CDemoStringTables{}
	for _, name := range s.order {
		out.Tables = append(out.Tables, s.tables[name])
	}
	return out
}

// segment is a tick range [from, to] of one source demo.
type segment struct {
	src      *demo
	from, to uint32
}

// writer emits a PBDEMS2 container.
type writer struct {
	buf bytes.Buffer
}

func (w *writer) put(kind protocol.EDemoCommands, compressed bool, tick uint32, raw []byte) {
	k := uint64(kind)
	if compressed {
		k |= uint64(protocol.EDemoCommands_DEM_IsCompressed)
	}
	var tmp [binary.MaxVarintLen64]byte
	w.buf.Write(tmp[:binary.PutUvarint(tmp[:], k)])
	w.buf.Write(tmp[:binary.PutUvarint(tmp[:], uint64(tick))])
	w.buf.Write(tmp[:binary.PutUvarint(tmp[:], uint64(len(raw)))])
	w.buf.Write(raw)
}

// putPayload writes a decoded payload, compressing it when it is large.
func (w *writer) putPayload(kind protocol.EDemoCommands, tick uint32, payload []byte) {
	if len(payload) > 512 {
		w.put(kind, true, tick, snappy.Encode(nil, payload))
		return
	}
	w.put(kind, false, tick, payload)
}

type options struct {
	stripUserCmds bool
}

func clip(segs []segment, opt options) []byte {
	w := &writer{}
	w.buf.WriteString("PBDEMS2\x00")
	w.buf.Write(make([]byte, 8))
	for _, c := range segs[0].src.signon {
		w.put(c.kind, c.compressed, c.tick, c.raw)
	}
	fp := segs[0].src.fingerprint()
	var out uint32 // next output tick
	frames := 0
	for i, seg := range segs {
		if seg.src.fingerprint() != fp {
			panic(fmt.Sprintf("segment %d: send tables or classes differ from the first segment's", i))
		}
		if i > 0 {
			out++
		}
		base := out
		tables := &tableState{tables: map[string]*protocol.CDemoStringTablesTableT{}}
		for _, c := range seg.src.signon {
			if c.kind == protocol.EDemoCommands_DEM_StringTables {
				st := &protocol.CDemoStringTables{}
				must(st.UnmarshalVT(c.payload()))
				tables.apply(st)
			}
		}
		var last *protocol.CDemoFullPacket
		var prefix [][]byte
		started := false
		for _, c := range seg.src.game {
			if c.tick == preGameTick {
				continue
			}
			if c.tick > seg.to {
				break
			}
			if !started && c.tick < seg.from {
				switch c.kind {
				case protocol.EDemoCommands_DEM_FullPacket:
					f := &protocol.CDemoFullPacket{}
					must(f.UnmarshalVT(c.payload()))
					tables.apply(f.GetStringTable())
					last = f
					prefix = nil
				case protocol.EDemoCommands_DEM_Packet:
					if last != nil {
						p := &protocol.CDemoPacket{}
						must(p.UnmarshalVT(c.payload()))
						if data := filterPacket(p.GetData(), keepState); len(data) > 0 {
							prefix = append(prefix, data)
						}
					}
				}
				continue
			}
			if !started {
				started = true
				if c.kind != protocol.EDemoCommands_DEM_FullPacket {
					if last == nil {
						panic(fmt.Sprintf("segment %d: no full packet at or before tick %d", i, seg.from))
					}
					synth := &protocol.CDemoFullPacket{StringTable: tables.snapshot(), Packet: last.GetPacket()}
					payload, err := synth.MarshalVT()
					must(err)
					w.putPayload(protocol.EDemoCommands_DEM_FullPacket, base, payload)
					for _, data := range prefix {
						payload, err := (&protocol.CDemoPacket{Data: data}).MarshalVT()
						must(err)
						w.putPayload(protocol.EDemoCommands_DEM_Packet, base, payload)
					}
				}
			}
			t := base + (c.tick - seg.from)
			switch {
			case c.kind == protocol.EDemoCommands_DEM_FullPacket && t == base:
				// The segment starts on a full packet: complete its tables.
				f := &protocol.CDemoFullPacket{}
				must(f.UnmarshalVT(c.payload()))
				tables.apply(f.GetStringTable())
				f.StringTable = tables.snapshot()
				payload, err := f.MarshalVT()
				must(err)
				w.putPayload(c.kind, t, payload)
			case c.kind == protocol.EDemoCommands_DEM_Packet && opt.stripUserCmds:
				p := &protocol.CDemoPacket{}
				must(p.UnmarshalVT(c.payload()))
				payload, err := (&protocol.CDemoPacket{Data: filterPacket(p.GetData(), dropUserCmds)}).MarshalVT()
				must(err)
				w.putPayload(c.kind, t, payload)
			default:
				w.put(c.kind, c.compressed, t, c.raw)
			}
			if c.kind == protocol.EDemoCommands_DEM_Packet {
				frames++
			}
			out = t
		}
	}
	w.put(protocol.EDemoCommands_DEM_Stop, false, out, nil)
	spawnOff := w.buf.Len()
	if sg := segs[0].src.spawnGroups; sg != nil {
		w.put(sg.kind, sg.compressed, out, sg.raw)
	}
	infoOff := w.buf.Len()
	info := &protocol.CDemoFileInfo{}
	if src := segs[0].src.fileInfo; src != nil {
		info.GameInfo = src.GetGameInfo()
	}
	interval := float32(1.0 / 64)
	if src := segs[0].src.fileInfo; src != nil && src.GetPlaybackTicks() > 0 {
		interval = src.GetPlaybackTime() / float32(src.GetPlaybackTicks())
	}
	info.PlaybackTicks = ptr(int32(out))
	info.PlaybackFrames = ptr(int32(frames))
	info.PlaybackTime = ptr(float32(out) * interval)
	payload, err := info.MarshalVT()
	must(err)
	w.put(protocol.EDemoCommands_DEM_FileInfo, false, out, payload)
	b := w.buf.Bytes()
	binary.LittleEndian.PutUint32(b[8:12], uint32(infoOff))
	binary.LittleEndian.PutUint32(b[12:16], uint32(spawnOff))
	return b
}

func ptr[T any](v T) *T { return &v }

// keepState keeps the messages that carry world state, dropping user
// messages, game events, sounds, recorded inputs and voice, which would all
// fire at once when the prefix is applied in one frame.
func keepState(kind uint32) bool {
	switch kind {
	case uint32(protocol.SVC_Messages_svc_UserCmds), uint32(protocol.SVC_Messages_svc_VoiceData),
		uint32(protocol.SVC_Messages_svc_Sounds), uint32(protocol.SVC_Messages_svc_HLTVStatus),
		uint32(protocol.SVC_Messages_svc_Print), uint32(protocol.SVC_Messages_svc_UserMessage):
		return false
	}
	return kind < 100
}

func dropUserCmds(kind uint32) bool { return kind != uint32(protocol.SVC_Messages_svc_UserCmds) }

// filterPacket rewrites a packet's bit-packed message stream, keeping each
// message whose kind keep accepts.
func filterPacket(data []byte, keep func(uint32) bool) []byte {
	r := &bitReader{buf: data}
	bw := &bitWriter{}
	for r.remaining() > 8 {
		kind := r.ubitvar()
		size := r.varint()
		body := r.bytes(int(size))
		if keep(kind) {
			bw.ubitvar(kind)
			bw.varint(size)
			for _, b := range body {
				bw.bits(uint32(b), 8)
			}
		}
	}
	return bw.finish()
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
func (r *bitReader) bytes(n int) []byte {
	out := make([]byte, n)
	if r.pos&7 == 0 {
		copy(out, r.buf[r.pos>>3:])
		r.pos += n * 8
		return out
	}
	for i := range out {
		out[i] = byte(r.bits(8))
	}
	return out
}
func (r *bitReader) ubitvar() uint32 {
	v := r.bits(6)
	switch v & 0x30 {
	case 0x10:
		v = (v & 15) | r.bits(4)<<4
	case 0x20:
		v = (v & 15) | r.bits(8)<<4
	case 0x30:
		v = (v & 15) | r.bits(28)<<4
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

type bitWriter struct {
	buf []byte
	pos int
}

func (w *bitWriter) bits(v uint32, n int) {
	for i := 0; i < n; i++ {
		if w.pos>>3 == len(w.buf) {
			w.buf = append(w.buf, 0)
		}
		w.buf[w.pos>>3] |= byte(v>>i&1) << (w.pos & 7)
		w.pos++
	}
}
func (w *bitWriter) ubitvar(v uint32) {
	switch {
	case v < 16:
		w.bits(v, 6)
	case v < 256:
		w.bits(v&15|0x10, 6)
		w.bits(v>>4, 4)
	case v < 4096:
		w.bits(v&15|0x20, 6)
		w.bits(v>>4, 8)
	default:
		w.bits(v&15|0x30, 6)
		w.bits(v>>4, 28)
	}
}
func (w *bitWriter) varint(v uint32) {
	for {
		b := v & 0x7f
		v >>= 7
		if v != 0 {
			w.bits(b|0x80, 8)
			continue
		}
		w.bits(b, 8)
		return
	}
}
func (w *bitWriter) finish() []byte { return w.buf }

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	outPath := flag.String("o", "", "output demo")
	strip := flag.Bool("strip-usercmds", false, "drop recorded player inputs from the clip")
	flag.Parse()
	if *outPath == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: democlip -o out.dem src.dem:A-B [src.dem:C-D ...]")
		os.Exit(2)
	}
	cache := map[string]*demo{}
	var segs []segment
	for _, arg := range flag.Args() {
		i := strings.LastIndexByte(arg, ':')
		path := arg[:i]
		fromS, toS, _ := strings.Cut(arg[i+1:], "-")
		from, err := strconv.ParseUint(fromS, 10, 32)
		must(err)
		to := uint64(math.MaxUint32 - 1)
		if toS != "" {
			to, err = strconv.ParseUint(toS, 10, 32)
			must(err)
		}
		if cache[path] == nil {
			cache[path] = readDemo(path)
		}
		segs = append(segs, segment{src: cache[path], from: uint32(from), to: uint32(to)})
	}
	must(os.WriteFile(*outPath, clip(segs, options{stripUserCmds: *strip}), 0o644))
}
