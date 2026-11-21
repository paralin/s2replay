// Command demolayout prints the outer command layout of a PBDEMS2 demo: the
// header offsets, every non-packet command, the ticks of full packets, and a
// histogram of inner packet message kinds. It is an experiment for cutting
// and joining demos.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"

	"github.com/klauspost/compress/snappy"
	"github.com/paralin/s2replay/protocol"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: demolayout <demo.dem> [-v]")
		os.Exit(2)
	}
	verbose := len(os.Args) > 2
	buf, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Printf("magic=%q fileinfo_off=%d spawngroups_off=%d size=%d\n",
		buf[:8], binary.LittleEndian.Uint32(buf[8:12]), binary.LittleEndian.Uint32(buf[12:16]), len(buf))
	pos := 16
	kinds := map[string]int{}
	kindBytes := map[string]int{}
	var fullTicks []uint32
	var lastTick uint32
	counts := map[string]int{}
	for pos < len(buf) {
		start := pos
		raw, n := binary.Uvarint(buf[pos:])
		pos += n
		tick, n := binary.Uvarint(buf[pos:])
		pos += n
		size, n := binary.Uvarint(buf[pos:])
		pos += n
		payload := buf[pos : pos+int(size)]
		pos += int(size)
		kind := int32(raw)
		comp := kind&int32(protocol.EDemoCommands_DEM_IsCompressed) != 0
		kind &^= int32(protocol.EDemoCommands_DEM_IsCompressed)
		name := protocol.EDemoCommands(kind).String()
		counts[name]++
		if comp {
			payload, err = snappy.Decode(nil, payload)
			if err != nil {
				panic(err)
			}
		}
		t := uint32(tick)
		if t != ^uint32(0) {
			lastTick = t
		}
		switch protocol.EDemoCommands(kind) {
		case protocol.EDemoCommands_DEM_Packet, protocol.EDemoCommands_DEM_SignonPacket:
			var pkt protocol.CDemoPacket
			must(pkt.UnmarshalVT(payload))
			if len(fullTicks) == 1 && counts[name] < 4 {
				peDump = 1
			}
			walk(pkt.GetData(), kinds, kindBytes)
			if kind == int32(protocol.EDemoCommands_DEM_SignonPacket) || verbose {
				fmt.Printf("@%d %s tick=%d size=%d\n", start, name, int32(t), size)
			}
		case protocol.EDemoCommands_DEM_FullPacket:
			var fp protocol.CDemoFullPacket
			must(fp.UnmarshalVT(payload))
			fullTicks = append(fullTicks, t)
			sub := map[string]int{}
			if len(fullTicks) < 4 {
				peDump = 1
			}
			walk(fp.GetPacket().GetData(), sub, map[string]int{})
			var tn []string
			for _, tb := range fp.GetStringTable().GetTables() {
				tn = append(tn, fmt.Sprintf("%s:%d", tb.GetTableName(), len(tb.GetItems())))
			}
			fmt.Printf("@%d %s tick=%d size=%d tables=%v packet=%v\n", start, name, t, size, tn, sub)
		case protocol.EDemoCommands_DEM_StringTables:
			var st protocol.CDemoStringTables
			must(st.UnmarshalVT(payload))
			for _, tb := range st.GetTables() {
				fmt.Printf("  signon table %s:%d\n", tb.GetTableName(), len(tb.GetItems()))
			}
			fallthrough
		default:
			fmt.Printf("@%d %s tick=%d size=%d comp=%v\n", start, name, int32(t), size, comp)
		}
	}
	fmt.Printf("last tick %d, commands %v\n", lastTick, counts)
	fmt.Printf("full packets %d at %v\n", len(fullTicks), fullTicks)
	type kv struct {
		k    string
		n, b int
	}
	var all []kv
	for k, n := range kinds {
		all = append(all, kv{k, n, kindBytes[k]})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].b > all[j].b })
	for _, e := range all {
		fmt.Printf("%8d %10d %s\n", e.n, e.b, e.k)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var peDump = 0

// walk counts the inner messages of a packet payload by name.
func walk(data []byte, kinds, bytes map[string]int) {
	r := &bitReader{buf: data}
	for r.remaining() > 8 {
		k := r.ubitvar()
		sz := r.varint()
		r.skip(int(sz) * 8)
		name := msgName(int32(k))
		if k == uint32(protocol.SVC_Messages_svc_PacketEntities) && peDump > 0 {
			var pe protocol.CSVCMsg_PacketEntities
			must(pe.UnmarshalVT(r.buf[r.pos/8-int(sz) : r.pos/8]))
			fmt.Printf("    PE max=%d upd=%d delta=%v legacy_delta=%v baseline=%d upd_base=%v tick=%d pending_full=%v bytes=%d\n", pe.GetMaxEntries(), pe.GetUpdatedEntries(), pe.DeltaFrom, pe.GetLegacyIsDelta(), pe.GetBaseline(), pe.GetUpdateBaseline(), pe.GetServerTick(), pe.GetPendingFullFrame(), len(pe.GetEntityData()))
			peDump--
		}
		kinds[name]++
		bytes[name] += int(sz)
	}
}

func msgName(k int32) string {
	for _, m := range []map[int32]string{
		protocol.NET_Messages_name, protocol.SVC_Messages_name, protocol.EBaseUserMessages_name,
		protocol.CitadelUserMessageIds_name, protocol.EBaseGameEvents_name, protocol.ECitadelGameEvents_name,
		protocol.EBaseEntityMessages_name, protocol.CitadelEntityMessageIds_name,
	} {
		if n, ok := m[k]; ok {
			return n
		}
	}
	return fmt.Sprintf("kind%d", k)
}

type bitReader struct {
	buf []byte
	pos int
}

func (r *bitReader) remaining() int { return len(r.buf)*8 - r.pos }
func (r *bitReader) bits(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		b := (r.buf[r.pos>>3] >> (r.pos & 7)) & 1
		v |= uint32(b) << i
		r.pos++
	}
	return v
}
func (r *bitReader) skip(n int) { r.pos += n }
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
