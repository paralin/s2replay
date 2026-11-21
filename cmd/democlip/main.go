// Command democlip cuts tick ranges out of PBDEMS2 demos and joins them into
// one demo that the game plays from its first tick.
//
//	democlip -o out.dem src.dem:A-B [src2.dem:C-D ...]
//
// The game client builds its first frame from ordinary packets and reads a
// full packet only when seeking, so each segment opens with one packet that
// rebuilds the state at A: the entity frame of the source's last full packet
// before A, and an update of every string table to that moment (a demo may
// create each table only once). The source's state messages between that full
// packet and A follow at the same tick, then a full packet for seeks, then
// the commands from A to B. Game ticks in net_Tick and svc_PacketEntities are
// renumbered to follow the output's ticks, which rise from zero across every
// segment, because the client stalls on a game tick that runs backward.
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

// tableSchema is how the client numbers a string table and encodes its
// entries, as the table's svc_CreateStringTable in the signon set them.
type tableSchema struct {
	id       int32
	fixed    bool
	sizeBits int32
	flags    int32
	varint   bool
}

// tableSchemas reads the string tables the signon creates; ids follow the
// order of creation.
func (d *demo) tableSchemas() map[string]tableSchema {
	out := map[string]tableSchema{}
	for _, c := range d.signon {
		if c.kind != protocol.EDemoCommands_DEM_SignonPacket {
			continue
		}
		p := &protocol.CDemoPacket{}
		must(p.UnmarshalVT(c.payload()))
		eachMsg(p.GetData(), func(kind uint32, body []byte) {
			switch kind {
			case uint32(protocol.SVC_Messages_svc_ClearAllStringTables):
				clear(out)
			case uint32(protocol.SVC_Messages_svc_CreateStringTable):
				m := &protocol.CSVCMsg_CreateStringTable{}
				must(m.UnmarshalVT(body))
				out[m.GetName()] = tableSchema{
					id: int32(len(out)), fixed: m.GetUserDataFixedSize(), sizeBits: m.GetUserDataSizeBits(),
					flags: m.GetFlags(), varint: m.GetUsingVarintBitcounts(),
				}
			}
		})
	}
	return out
}

// rewriteTables returns a message stream of svc_UpdateStringTable messages
// that set every entry of each table in st, by index, to st's key and value.
// A demo may create each table only once, so a clip moves the client's tables
// to a segment's state by updating them. Entries past st's length, and values
// st leaves empty, keep what the client had: an update cannot remove them.
func rewriteTables(st *protocol.CDemoStringTables, schemas map[string]tableSchema) []byte {
	out := &bitWriter{}
	for _, t := range st.GetTables() {
		s, ok := schemas[t.GetTableName()]
		if !ok || len(t.GetItems()) == 0 {
			continue
		}
		e := &bitWriter{}
		for _, it := range t.GetItems() {
			e.bits(1, 1) // next index
			e.bits(1, 1) // has key
			e.bits(0, 1) // no key history
			for _, b := range []byte(it.GetStr()) {
				e.bits(uint32(b), 8)
			}
			e.bits(0, 8)
			data := it.GetData()
			if len(data) == 0 {
				e.bits(0, 1)
				continue
			}
			e.bits(1, 1)
			n := len(data) * 8
			if s.fixed {
				n = int(s.sizeBits)
			} else {
				if s.flags&1 != 0 {
					e.bits(0, 1) // not compressed
				}
				if s.varint {
					e.ubitvar(uint32(len(data)))
				} else {
					e.bits(uint32(len(data)), 17)
				}
			}
			for i := 0; i < n; i += 8 {
				var b byte // a fixed-size value shorter than its size pads with zeros
				if i/8 < len(data) {
					b = data[i/8]
				}
				e.bits(uint32(b), min(8, n-i))
			}
		}
		body, err := (&protocol.CSVCMsg_UpdateStringTable{
			TableId: ptr(s.id), NumChangedEntries: ptr(int32(len(t.GetItems()))), StringData: e.finish(),
		}).MarshalVT()
		must(err)
		out.ubitvar(uint32(protocol.SVC_Messages_svc_UpdateStringTable))
		out.varint(uint32(len(body)))
		for _, b := range body {
			out.bits(uint32(b), 8)
		}
	}
	return out.finish()
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

// putPacket writes a DEM_Packet holding the bit-packed messages data.
func (w *writer) putPacket(tick uint32, data []byte) {
	payload, err := (&protocol.CDemoPacket{Data: data}).MarshalVT()
	must(err)
	w.putPayload(protocol.EDemoCommands_DEM_Packet, tick, payload)
}

type options struct {
	stripUserCmds bool
}

func clip(segs []segment, opt options) []byte {
	w := &writer{}
	w.buf.WriteString("PBDEMS2\x00")
	w.buf.Write(make([]byte, 8))
	start := segs[0].src.serverStart() // game tick = start + output tick
	for _, c := range segs[0].src.signon {
		w.put(c.kind, c.compressed, c.tick, c.raw)
	}
	fp := segs[0].src.fingerprint()
	schemas := segs[0].src.tableSchemas()
	var out uint32 // last output tick
	frames := 0
	for i, seg := range segs {
		if seg.src.fingerprint() != fp {
			panic(fmt.Sprintf("segment %d: send tables or classes differ from the first segment's", i))
		}
		// The opening sits one tick before the segment's first tick and
		// after the previous segment's last.
		open := out + 1
		if i == 0 {
			open = 0
		}
		base := open + 1
		// Game ticks follow output ticks: the client orders frames by them
		// and stalls when one runs backward. The opening's frames all take
		// the opening's tick, so each prefix delta still applies to the frame
		// before it.
		first := seg.src.serverStart() + seg.from
		retick := func(old uint32) uint32 {
			if old < first {
				return start + open
			}
			return start + base + (old - first)
		}
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
			if c.tick < seg.from {
				switch c.kind {
				case protocol.EDemoCommands_DEM_FullPacket:
					f := &protocol.CDemoFullPacket{}
					must(f.UnmarshalVT(c.payload()))
					tables.apply(f.GetStringTable())
					last, prefix = f, nil
				case protocol.EDemoCommands_DEM_Packet:
					if last != nil {
						p := &protocol.CDemoPacket{}
						must(p.UnmarshalVT(c.payload()))
						if data := rewrite(p.GetData(), keepState, retick); len(data) > 0 {
							prefix = append(prefix, data)
						}
					}
				}
				continue
			}
			if !started {
				started = true
				if last == nil {
					panic(fmt.Sprintf("segment %d: no full packet before tick %d", i, seg.from))
				}
				// The client applies a full packet only when seeking; in
				// playback it builds its first frame from ordinary packets,
				// and it ends the demo at a packet without a net_Tick. So the
				// opening is one packet: the last full packet's entity frame
				// with every string table rewritten to that moment, then the
				// state messages since. The full packet follows, for seeks.
				frame := rewrite(last.GetPacket().GetData(), keepAll, retick)
				w.putPacket(open, insertAfterTick(frame, rewriteTables(tables.snapshot(), schemas)))
				synth := &protocol.CDemoFullPacket{StringTable: tables.snapshot(), Packet: &protocol.CDemoPacket{Data: frame}}
				payload, err := synth.MarshalVT()
				must(err)
				w.putPayload(protocol.EDemoCommands_DEM_FullPacket, open, payload)
				for _, data := range prefix {
					w.putPacket(open, data)
				}
			}
			t := base + (c.tick - seg.from)
			switch c.kind {
			case protocol.EDemoCommands_DEM_Packet:
				p := &protocol.CDemoPacket{}
				must(p.UnmarshalVT(c.payload()))
				keep := keepAll
				if opt.stripUserCmds {
					keep = dropUserCmds
				}
				w.putPacket(t, rewrite(p.GetData(), keep, retick))
				frames++
			case protocol.EDemoCommands_DEM_FullPacket:
				f := &protocol.CDemoFullPacket{}
				must(f.UnmarshalVT(c.payload()))
				f.Packet = &protocol.CDemoPacket{Data: rewrite(f.GetPacket().GetData(), keepAll, retick)}
				payload, err := f.MarshalVT()
				must(err)
				w.putPayload(c.kind, t, payload)
			default:
				w.put(c.kind, c.compressed, t, c.raw)
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

// serverStart returns the header's server_start_tick, the game tick of the
// demo's tick zero.
func (d *demo) serverStart() uint32 {
	for _, c := range d.signon {
		if c.kind == protocol.EDemoCommands_DEM_FileHeader {
			h := &protocol.CDemoFileHeader{}
			must(h.UnmarshalVT(c.payload()))
			return uint32(h.GetServerStartTick())
		}
	}
	return 0
}

// rewrite rewrites a packet's bit-packed message stream: it keeps each
// message whose kind keep accepts, and moves the game ticks that net_Tick
// and svc_PacketEntities carry through retick.
func rewrite(data []byte, keep func(uint32) bool, retick func(uint32) uint32) []byte {
	bw := &bitWriter{}
	eachMsg(data, func(kind uint32, body []byte) {
		if !keep(kind) {
			return
		}
		switch kind {
		case uint32(protocol.NET_Messages_net_Tick):
			m := &protocol.CNETMsg_Tick{}
			must(m.UnmarshalVT(body))
			m.Tick = ptr(retick(m.GetTick()))
			var err error
			body, err = m.MarshalVT()
			must(err)
		case uint32(protocol.SVC_Messages_svc_PacketEntities):
			m := &protocol.CSVCMsg_PacketEntities{}
			must(m.UnmarshalVT(body))
			m.ServerTick = ptr(retick(m.GetServerTick()))
			if m.DeltaFrom != nil {
				m.DeltaFrom = ptr(int32(retick(uint32(m.GetDeltaFrom()))))
			}
			var err error
			body, err = m.MarshalVT()
			must(err)
		}
		bw.ubitvar(kind)
		bw.varint(uint32(len(body)))
		for _, b := range body {
			bw.bits(uint32(b), 8)
		}
	})
	return bw.finish()
}

func keepAll(uint32) bool { return true }

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

// insertAfterTick returns data's message stream with the messages of extra
// inserted after its net_Tick, so they apply in the same frame.
func insertAfterTick(data []byte, extra ...[]byte) []byte {
	bw := &bitWriter{}
	copyMsgs := func(data []byte, after func(kind uint32)) {
		r := &bitReader{buf: data}
		for r.remaining() > 8 {
			kind := r.ubitvar()
			size := r.varint()
			body := r.bytes(int(size))
			bw.ubitvar(kind)
			bw.varint(size)
			for _, b := range body {
				bw.bits(uint32(b), 8)
			}
			after(kind)
		}
	}
	copyMsgs(data, func(kind uint32) {
		if kind == uint32(protocol.NET_Messages_net_Tick) {
			for _, e := range extra {
				copyMsgs(e, func(uint32) {})
			}
		}
	})
	return bw.finish()
}

// eachMsg calls fn with the kind and body of each message in a packet's
// bit-packed message stream.
func eachMsg(data []byte, fn func(kind uint32, body []byte)) {
	r := &bitReader{buf: data}
	for r.remaining() > 8 {
		kind := r.ubitvar()
		size := r.varint()
		fn(kind, r.bytes(int(size)))
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
