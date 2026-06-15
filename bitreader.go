package s2replay

import (
	"encoding/binary"
	"math"
)

// packetReader reads the packet layer inside CDemoPacket.data. Packet message
// ids use Valve ubitvar encoding, followed by byte-varint sizes and protobuf
// payload bytes.
type packetReader struct {
	buf      []byte
	pos      int
	bitVal   uint64
	bitCount uint8
}

func newPacketReader(buf []byte) *packetReader { return &packetReader{buf: buf} }

func (r *packetReader) bitsRemaining() int {
	return (len(r.buf)-r.pos)*8 + int(r.bitCount)
}

func (r *packetReader) readBits(n uint8) (uint32, error) {
	if n > 32 {
		return 0, errBitReadOverflow
	}
	for n > r.bitCount {
		if r.pos >= len(r.buf) {
			return 0, errBitReadOverflow
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

func (r *packetReader) readByte() (byte, error) {
	if r.bitCount == 0 {
		if r.pos >= len(r.buf) {
			return 0, errShortRead
		}
		b := r.buf[r.pos]
		r.pos++
		return b, nil
	}
	v, err := r.readBits(8)
	return byte(v), err
}

func (r *packetReader) readBool() (bool, error) {
	v, err := r.readBits(1)
	return v == 1, err
}

func (r *packetReader) readBytes(n int) ([]byte, error) {
	if n < 0 {
		return nil, errNegativePacketSize
	}
	if r.bitCount == 0 {
		if n > len(r.buf)-r.pos {
			return nil, errShortRead
		}
		b := r.buf[r.pos : r.pos+n]
		r.pos += n
		return b, nil
	}

	if n*8 > r.bitsRemaining() {
		return nil, errShortRead
	}
	b := make([]byte, n)
	for i := range b {
		v, err := r.readByte()
		if err != nil {
			return nil, err
		}
		b[i] = v
	}
	return b, nil
}

func (r *packetReader) readBitsAsBytes(bits int) ([]byte, error) {
	if bits < 0 {
		return nil, errShortRead
	}
	b := make([]byte, 0, (bits+7)/8)
	for bits >= 8 {
		v, err := r.readByte()
		if err != nil {
			return nil, err
		}
		b = append(b, v)
		bits -= 8
	}
	if bits > 0 {
		v, err := r.readBits(uint8(bits))
		if err != nil {
			return nil, err
		}
		b = append(b, byte(v))
	}
	return b, nil
}

func (r *packetReader) readLEUint32() (uint32, error) {
	b, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *packetReader) readLEUint64() (uint64, error) {
	b, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (r *packetReader) readUvarint32() (uint32, error) {
	var x uint32
	var s uint
	for i := 0; i < 5; i++ {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if b < 0x80 {
			if i == 4 && b > 0x0f {
				return 0, errInvalidVarint
			}
			return x | uint32(b)<<s, nil
		}
		x |= uint32(b&0x7f) << s
		s += 7
	}
	return 0, errInvalidVarint
}

func (r *packetReader) readVarint32() (int32, error) {
	v, err := r.readUvarint32()
	if err != nil {
		return 0, err
	}
	x := int32(v >> 1)
	if v&1 != 0 {
		x = ^x
	}
	return x, nil
}

func (r *packetReader) readUvarint64() (uint64, error) {
	var x uint64
	var s uint
	for i := 0; i < 10; i++ {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if b < 0x80 {
			if i == 9 && b > 1 {
				return 0, errInvalidVarint
			}
			return x | uint64(b)<<s, nil
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, errInvalidVarint
}

func (r *packetReader) readUBitVar() (uint32, error) {
	v, err := r.readBits(6)
	if err != nil {
		return 0, err
	}

	switch v & 0x30 {
	case 0x10:
		extra, err := r.readBits(4)
		if err != nil {
			return 0, err
		}
		return (v & 0x0f) | extra<<4, nil
	case 0x20:
		extra, err := r.readBits(8)
		if err != nil {
			return 0, err
		}
		return (v & 0x0f) | extra<<4, nil
	case 0x30:
		extra, err := r.readBits(28)
		if err != nil {
			return 0, err
		}
		return (v & 0x0f) | extra<<4, nil
	default:
		return v, nil
	}
}

func (r *packetReader) readUBitVarFieldPath() (int, error) {
	v, err := r.readBool()
	if err != nil || v {
		if err != nil {
			return 0, err
		}
		x, err := r.readBits(2)
		return int(x), err
	}
	v, err = r.readBool()
	if err != nil || v {
		if err != nil {
			return 0, err
		}
		x, err := r.readBits(4)
		return int(x), err
	}
	v, err = r.readBool()
	if err != nil || v {
		if err != nil {
			return 0, err
		}
		x, err := r.readBits(10)
		return int(x), err
	}
	v, err = r.readBool()
	if err != nil || v {
		if err != nil {
			return 0, err
		}
		x, err := r.readBits(17)
		return int(x), err
	}
	x, err := r.readBits(31)
	return int(x), err
}

func (r *packetReader) readString() (string, error) {
	b := make([]byte, 0, 32)
	for {
		c, err := r.readByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			return string(b), nil
		}
		b = append(b, c)
	}
}

func (r *packetReader) readFloat32() (float32, error) {
	v, err := r.readLEUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func (r *packetReader) readCoord() (float32, error) {
	intval, err := r.readBits(1)
	if err != nil {
		return 0, err
	}
	fractval, err := r.readBits(1)
	if err != nil {
		return 0, err
	}
	if intval == 0 && fractval == 0 {
		return 0, nil
	}
	neg, err := r.readBool()
	if err != nil {
		return 0, err
	}
	if intval != 0 {
		intval, err = r.readBits(14)
		if err != nil {
			return 0, err
		}
		intval++
	}
	if fractval != 0 {
		fractval, err = r.readBits(5)
		if err != nil {
			return 0, err
		}
	}
	v := float32(intval) + float32(fractval)*(1.0/(1<<5))
	if neg {
		v = -v
	}
	return v, nil
}

func (r *packetReader) readAngle(n uint8) (float32, error) {
	v, err := r.readBits(n)
	if err != nil {
		return 0, err
	}
	return float32(v) * 360.0 / float32(uint32(1)<<n), nil
}

func (r *packetReader) readNormal() (float32, error) {
	neg, err := r.readBool()
	if err != nil {
		return 0, err
	}
	v, err := r.readBits(11)
	if err != nil {
		return 0, err
	}
	ret := float32(v) * float32(1.0/(float32(1<<11)-1.0))
	if neg {
		ret = -ret
	}
	return ret, nil
}

func (r *packetReader) read3BitNormal() ([3]float32, error) {
	var ret [3]float32
	hasX, err := r.readBool()
	if err != nil {
		return ret, err
	}
	hasY, err := r.readBool()
	if err != nil {
		return ret, err
	}
	if hasX {
		ret[0], err = r.readNormal()
		if err != nil {
			return ret, err
		}
	}
	if hasY {
		ret[1], err = r.readNormal()
		if err != nil {
			return ret, err
		}
	}
	negZ, err := r.readBool()
	if err != nil {
		return ret, err
	}
	prod := ret[0]*ret[0] + ret[1]*ret[1]
	if prod < 1 {
		ret[2] = float32(math.Sqrt(float64(1 - prod)))
	}
	if negZ {
		ret[2] = -ret[2]
	}
	return ret, nil
}
