// Command kind145track extracts subtype-27 transform updates (per-tick entity
// origins and orientations) from Deadlock demo inner-packet message kind 145.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"

	snappy "github.com/klauspost/compress/snappy"
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
		x |= uint32(b&0x7f) << s
		s += 7
	}
	return 0, fmt.Errorf("bad varint")
}

// pbField is one raw protobuf field.
type pbField struct {
	num    int
	wire   int
	varint uint64
	fixed  []byte
	data   []byte
}

func walkPB(buf []byte) ([]pbField, error) {
	var out []pbField
	pos := 0
	for pos < len(buf) {
		key, n := binary.Uvarint(buf[pos:])
		if n <= 0 {
			return out, fmt.Errorf("bad key at %d", pos)
		}
		pos += n
		f := pbField{num: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(buf[pos:])
			if n <= 0 {
				return out, fmt.Errorf("bad varint")
			}
			f.varint = v
			pos += n
		case 1:
			if pos+8 > len(buf) {
				return out, fmt.Errorf("short fixed64")
			}
			f.fixed = buf[pos : pos+8]
			pos += 8
		case 2:
			l, n := binary.Uvarint(buf[pos:])
			if n <= 0 || pos+n+int(l) > len(buf) {
				return out, fmt.Errorf("short bytes")
			}
			pos += n
			f.data = buf[pos : pos+int(l)]
			pos += int(l)
		case 5:
			if pos+4 > len(buf) {
				return out, fmt.Errorf("short fixed32")
			}
			f.fixed = buf[pos : pos+4]
			pos += 4
		default:
			return out, fmt.Errorf("wire %d", f.wire)
		}
		out = append(out, f)
	}
	return out, nil
}

func fnum(fs []pbField, num int) (pbField, bool) {
	for _, f := range fs {
		if f.num == num {
			return f, true
		}
	}
	return pbField{}, false
}

func fixed32s(b []byte) []float32 {
	var out []float32
	for i := 0; i+4 <= len(b); i += 4 {
		out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(b[i:])))
	}
	return out
}

type trackRec struct {
	Tick             uint32  `json:"tick"`
	Eid              uint64  `json:"eid"`
	X                float32 `json:"x"`
	Y                float32 `json:"y"`
	Z                float32 `json:"z"`
	Pitch            float32 `json:"pitch,omitempty"`
	Yaw              float32 `json:"yaw,omitempty"`
	Roll             float32 `json:"roll,omitempty"`
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

func main() {
	demoPath, outPath := os.Args[1], os.Args[2]
	demo, err := os.ReadFile(demoPath)
	if err != nil {
		panic(err)
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		panic(err)
	}
	out, _ := os.Create(outPath)
	w := bufio.NewWriter(out)
	defer func() { w.Flush(); out.Close() }()
	enc := json.NewEncoder(w)

	count := 0
	for {
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
		for (len(data)-r.pos)*8+int(r.bitCount) > 8 {
			kind, err := r.readUBitVar()
			if err != nil {
				break
			}
			size, err := r.readUvarint32()
			if err != nil {
				break
			}
			buf := make([]byte, size)
			ok := true
			for j := range buf {
				b, err := r.readByte()
				if err != nil {
					ok = false
					break
				}
				buf[j] = b
			}
			if !ok {
				break
			}
			if kind != 145 {
				continue
			}
			outer, err := walkPB(buf)
			if err != nil {
				continue
			}
			sub, ok1 := fnum(outer, 1)
			eidF, ok2 := fnum(outer, 2)
			if !ok1 || !ok2 || sub.varint != 27 {
				continue
			}
			tf, ok3 := fnum(outer, 30)
			if !ok3 {
				continue
			}
			inner, err := walkPB(tf.data)
			if err != nil {
				continue
			}
			rec := trackRec{Tick: cmd.Tick, Eid: eidF.varint}
			if origin, ok := fnum(inner, 2); ok && origin.wire == 2 {
				if v := fixed32s(origin.data); len(v) >= 3 {
					rec.X, rec.Y, rec.Z = v[0], v[1], v[2]
				} else {
					continue
				}
			} else {
				continue
			}
			if ang, ok := fnum(inner, 3); ok && ang.wire == 2 {
				if v := fixed32s(ang.data); len(v) >= 3 {
					rec.Pitch, rec.Yaw, rec.Roll = v[0], v[1], v[2]
				}
			}
			enc.Encode(rec)
			count++
		}
	}
	fmt.Println("tracked:", count)
}
