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
	"strconv"

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
	Tick       uint32  `json:"tick"`
	Eid        uint64  `json:"eid"`
	Subtype    uint64  `json:"subtype"`
	Steamid    uint64  `json:"steamid,omitempty"`
	UpdateType uint64  `json:"update_type,omitempty"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	Z          float32 `json:"z"`
	Pitch      float32 `json:"pitch,omitempty"`
	Yaw        float32 `json:"yaw,omitempty"`
	Roll       float32 `json:"roll,omitempty"`
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
	subCounts := map[uint64]int{}
	targetEid, _ := strconv.ParseUint(os.Getenv("K145_EID"), 10, 64)
	loTick, _ := strconv.ParseUint(os.Getenv("K145_LO"), 10, 64)
	hiTick, _ := strconv.ParseUint(os.Getenv("K145_HI"), 10, 64)
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
			if os.Getenv("K145_RAW") != "" {
				outerRaw, err := walkPB(buf)
				if err == nil {
					sub, _ := fnum(outerRaw, 1)
					eidF, _ := fnum(outerRaw, 2)
					if sub.varint == 27 && eidF.varint == targetEid && uint64(cmd.Tick) >= loTick && uint64(cmd.Tick) <= hiTick {
						fmt.Fprintf(os.Stderr, "tick %d eid %d: %x\n", cmd.Tick, eidF.varint, buf)
					}
				}
			}
			outer, err := walkPB(buf)
			if err != nil {
				continue
			}
			sub, ok1 := fnum(outer, 1)
			eidF, ok2 := fnum(outer, 2)
			if !ok1 || !ok2 {
				continue
			}
			subCounts[sub.varint]++
			// Subtype 5 carries player-keyed transforms: outer field 12 is a
			// body message with a steamid-like varint in its field 2 and a
			// tagged vec3 origin in its field 5.
			if sub.varint == 5 {
				if bodyF, okB := fnum(outer, 12); okB && bodyF.wire == 2 {
					body, errB := walkPB(bodyF.data)
					if errB != nil {
						continue
					}
					rec := trackRec{Tick: cmd.Tick, Eid: eidF.varint, Subtype: 5}
					if sid, okS := fnum(body, 2); okS && sid.wire == 0 {
						rec.Steamid = sid.varint
					}
					if pos, okP := fnum(body, 5); okP && pos.wire == 2 {
						fs, errP := walkPB(pos.data)
						if errP != nil {
							continue
						}
						got := 0
						for _, ff := range fs {
							if ff.num >= 1 && ff.num <= 3 && ff.wire == 5 && len(ff.fixed) == 4 {
								v := math.Float32frombits(binary.LittleEndian.Uint32(ff.fixed))
								switch ff.num {
								case 1:
									rec.X = v
								case 2:
									rec.Y = v
								case 3:
									rec.Z = v
								}
								got++
							}
						}
						if got == 3 {
							enc.Encode(rec)
							count++
						}
					}
				}
				continue
			}
			if sub.varint != 27 {
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
			if ut, ok := fnum(inner, 1); ok && ut.wire == 0 {
				rec.UpdateType = ut.varint
			}

			// Origin and angles are themselves protobuf messages whose
			// fields 1-3 are fixed32 floats; walking them keeps the field
			// tags from desyncing the triple.
			vec3 := func(f pbField) ([3]float32, bool) {
				var out [3]float32
				fs, err := walkPB(f.data)
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
			origin, okO := fnum(inner, 2)
			if !okO || origin.wire != 2 {
				continue
			}
			o, okOrigin := vec3(origin)
			if !okOrigin {
				continue
			}
			rec.X, rec.Y, rec.Z = o[0], o[1], o[2]
			if angles, okA := fnum(inner, 3); okA && angles.wire == 2 {
				if a, okAng := vec3(angles); okAng {
					rec.Pitch, rec.Yaw, rec.Roll = a[0], a[1], a[2]
				}
			}
			enc.Encode(rec)
			count++
		}
	}
	fmt.Println("tracked:", count)
	if os.Getenv("K145_SUBTYPES") != "" {
		for st, n := range subCounts {
			fmt.Fprintf(os.Stderr, "subtype %d: %d\n", st, n)
		}
	}
}
