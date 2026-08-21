
// Command kind145dump walks a PBDEMS2 demo and dumps raw inner-packet
// payloads of one message kind with per-tick context for offline analysis.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/snappy"
	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

const demoIsCompressed = int32(protocol.EDemoCommands_DEM_IsCompressed)

type bitReader struct {
	buf      []byte
	pos      int
	bitVal   uint64
	bitCount uint8
}

func (r *bitReader) readBits(n uint8) (uint32, error) {
	for n > r.bitCount {
		if r.pos >= len(r.buf) {
			return 0, fmt.Errorf("short read")
		}
		r.bitVal |= uint64(r.buf[r.pos]) << r.bitCount
		r.pos++
		r.bitCount += 8
	}
	mask := uint64(1<<n) - 1
	if n == 32 {
		mask = 1<<32 - 1
	}
	v := uint32(r.bitVal & mask)
	r.bitVal >>= n
	r.bitCount -= n
	return v, nil
}

func (r *bitReader) readByte() (byte, error) {
	if r.bitCount == 0 {
		if r.pos >= len(r.buf) {
			return 0, fmt.Errorf("short read")
		}
		b := r.buf[r.pos]
		r.pos++
		return b, nil
	}
	v, err := r.readBits(8)
	return byte(v), err
}

func (r *bitReader) readUvarint32() (uint32, error) {
	var x uint32
	var s uint
	for i := range 5 {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if b < 0x80 {
			if i == 4 && b > 0x0f {
				return 0, fmt.Errorf("bad varint")
			}
			return x | uint32(b)<<s, nil
		}
		x |= uint32(b & 0x7f) << s
		s += 7
	}
	return 0, fmt.Errorf("bad varint")
}

func (r *bitReader) readUBitVar() (uint32, error) {
	v, err := r.readBits(6)
	if err != nil {
		return 0, err
	}
	switch v & 0x30 {
	case 0x10:
		extra, err := r.readBits(4)
		return (v & 0x0f) | extra<<4, err
	case 0x20:
		extra, err := r.readBits(8)
		return (v & 0x0f) | extra<<4, err
	case 0x30:
		extra, err := r.readBits(28)
		return (v & 0x0f) | extra<<4, err
	default:
		return v, nil
	}
}

type msgRec struct {
	Tick   uint32 `json:"tick"`
	Packet int    `json:"packet"`
	Index  int    `json:"index"`
	Kind   uint32 `json:"kind"`
	Size   int    `json:"size"`
	File   string `json:"file,omitempty"`
}

func main() {
	demoPath, outDir := os.Args[1], os.Args[2]
	target := uint32(145)
	maxDumps := 400
	demo, err := os.ReadFile(demoPath)
	if err != nil {
		panic(err)
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		panic(err)
	}
	os.MkdirAll(outDir, 0o755)

	indexF, _ := os.Create(filepath.Join(outDir, "index.jsonl"))
	defer indexF.Close()
	enc := json.NewEncoder(indexF)

	dumped := 0
	packets := 0
	var hist = map[int]int{}
	tickHist := map[uint32]int{}

	for i := 0; ; i++ {
		cmd, err := p.Next()
		if err != nil {
			break
		}
		k := int32(cmd.Kind)
		compressed := k&demoIsCompressed != 0
		k &^= demoIsCompressed
		payload := cmd.Payload
		if compressed {
			payload, err = snappy.Decode(nil, payload)
			if err != nil {
				continue
			}
		}
		var data []byte
		switch protocol.EDemoCommands(k) {
		case protocol.EDemoCommands_DEM_Packet, protocol.EDemoCommands_DEM_SignonPacket:
			m := &protocol.CDemoPacket{}
			if m.UnmarshalVT(payload) != nil {
				continue
			}
			data = m.GetData()
		case protocol.EDemoCommands_DEM_FullPacket:
			m := &protocol.CDemoFullPacket{}
			if m.UnmarshalVT(payload) != nil || m.GetPacket() == nil {
				continue
			}
			data = m.GetPacket().GetData()
		default:
			continue
		}
		r := &bitReader{buf: data}
		idx := 0
		for (len(data)-r.pos)*8+int(r.bitCount) > 8 {
			kind, err := r.readUBitVar()
			if err != nil {
				break
			}
			size, err := r.readUvarint32()
			if err != nil {
				break
			}
			startBit := (r.pos - int(r.bitCount)/8)*8 + int(r.bitCount)%8
			buf := make([]byte, size)
			copyOK := true
			for j := range buf {
				b, err := r.readByte()
				if err != nil {
					copyOK = false
					break
				}
				buf[j] = b
			}
			if !copyOK {
				break
			}
			rec := msgRec{Tick: cmd.Tick, Packet: i, Index: idx, Kind: kind, Size: int(size)}
			if kind == target {
				hist[int(size)]++
				tickHist[cmd.Tick]++
				if dumped < maxDumps {
					name := fmt.Sprintf("k145_tick%d_pkt%d_idx%d_sz%d_bit%d.bin", cmd.Tick, i, idx, size, startBit)
					os.WriteFile(filepath.Join(outDir, name), buf, 0o644)
					rec.File = name
					dumped++
				}
				enc.Encode(rec)
			}
			idx++
		}
		packets++
	}
	summF, _ := os.Create(filepath.Join(outDir, "summary.json"))
	json.NewEncoder(summF).Encode(map[string]any{
		"packets": packets, "dumped": dumped,
		"sizeHistogram": hist,
		"ticksWithTarget": len(tickHist),
	})
	fmt.Println("done dumped:", dumped, "packets:", packets)
}
