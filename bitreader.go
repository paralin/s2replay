package s2replay

import (
	"encoding/binary"
	"math"
)

// packetReader reads the packet layer inside CDemoPacket.data. Packet message
// ids use Valve ubitvar encoding, followed by byte-varint sizes and protobuf
// payload bytes.
type packetReader struct {
	// buf is the borrowed packet payload.
	buf []byte
	// pos is the next payload byte to load into the accumulator.
	pos int
	// bitVal holds unread bits in its least significant positions.
	bitVal uint64
	// bitCount counts unread bits in bitVal.
	bitCount uint8
}

// newPacketReader constructs a bit reader over the packet payload bytes.
func newPacketReader(buf []byte) *packetReader { return &packetReader{buf: buf} }

// bitsRemaining returns the number of bits still readable from the packet
// payload, including partially consumed bytes held in the bit accumulator.
func (r *packetReader) bitsRemaining() int {
	return (len(r.buf)-r.pos)*8 + int(r.bitCount)
}

// readBits reads the next n bits (at most 32) as the low bits of a uint32,
// consuming buffered bits before pulling new bytes from the payload.
func (r *packetReader) readBits(n uint8) (uint32, error) {
	if n > 32 {
		return 0, errBitReadOverflow
	}

	// Refill the accumulator a byte at a time until the request is satisfied.
	for n > r.bitCount {
		if r.pos >= len(r.buf) {
			return 0, errBitReadOverflow
		}
		r.bitVal |= uint64(r.buf[r.pos]) << r.bitCount
		r.pos++
		r.bitCount += 8
	}

	// Mask the low n bits and retire them from the accumulator.
	mask := uint64(1<<n) - 1
	if n == 32 {
		mask = 1<<32 - 1
	}
	v := uint32(r.bitVal & mask)
	r.bitVal >>= n
	r.bitCount -= n
	return v, nil
}

// readByte reads the next whole byte, taking the fast byte-aligned path when
// no partially consumed bits are buffered.
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

// readBool reads the next single bit.
func (r *packetReader) readBool() (bool, error) {
	v, err := r.readBits(1)
	return v == 1, err
}

// readBytes reads n bytes, returning a view into the payload when the
// accumulator is empty and a copied read otherwise.
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

	// Misaligned reads cannot alias the payload, so copy byte by byte.
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

// readBitsAsBytes consumes exactly bits bits into a fresh byte slice.
// Unused high bits in the final output byte are zero; subsequent input stays unread.
func (r *packetReader) readBitsAsBytes(bits int) ([]byte, error) {
	if bits < 0 || bits > r.bitsRemaining() {
		return nil, errShortRead
	}
	b := make([]byte, 0, (bits+7)/8)

	// Consume whole bytes while aligned reads remain.
	for bits >= 8 {
		v, err := r.readByte()
		if err != nil {
			return nil, err
		}
		b = append(b, v)
		bits -= 8
	}

	// Read the trailing partial byte, if any.
	if bits > 0 {
		v, err := r.readBits(uint8(bits))
		if err != nil {
			return nil, err
		}
		b = append(b, byte(v))
	}
	return b, nil
}

// readLEUint32 reads a little-endian uint32.
func (r *packetReader) readLEUint32() (uint32, error) {
	b, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

// readLEUint64 reads a little-endian uint64.
func (r *packetReader) readLEUint64() (uint64, error) {
	b, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// readUvarint32 reads a Valve uvarint capped at 5 bytes, rejecting a fifth
// byte that would carry more value bits than a uint32 can hold.
func (r *packetReader) readUvarint32() (uint32, error) {
	var x uint32
	var s uint

	// Accumulate continuation bytes; the fifth byte may carry only 4 value bits.
	for i := range 5 {
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

// readVarint32 reads a Valve zigzag-encoded signed varint.
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

// readUvarint64 reads a Valve uvarint capped at 10 bytes, rejecting a tenth
// byte that would carry more value bits than a uint64 can hold.
func (r *packetReader) readUvarint64() (uint64, error) {
	var x uint64
	var s uint

	// Accumulate continuation bytes; the tenth byte may carry only 1 value bit.
	for i := range 10 {
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

// readUBitVar reads a Valve ubitvar: a 6-bit prefix whose top two bits select
// the width of a following continuation group.
func (r *packetReader) readUBitVar() (uint32, error) {
	v, err := r.readBits(6)
	if err != nil {
		return 0, err
	}

	// Decode the continuation group selected by the prefix's top two bits.
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

// readUBitVarFieldPath reads a Valve field-path encoding: flag bits select
// successively wider path components, ending at a 31-bit maximum.
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

// readString reads a NUL-terminated byte string.
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

// readStringMax reads a NUL-terminated byte string, rejecting input longer
// than maxBytes with the string-table key limit error.
func (r *packetReader) readStringMax(maxBytes int) (string, error) {
	b := make([]byte, 0, min(32, maxBytes))
	for {
		c, err := r.readByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			return string(b), nil
		}
		if len(b) == maxBytes {
			return "", errStringTableKeyTooLarge
		}
		b = append(b, c)
	}
}

// readFloat32 reads an IEEE 754 binary32 value.
func (r *packetReader) readFloat32() (float32, error) {
	v, err := r.readLEUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

// readCoord reads a Valve quantized coordinate: an integer part, a 5-bit
// fraction, and a sign bit, each optional in the wire encoding.
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

	// Read the sign, then the integer and fraction components that are present.
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

	// Assemble the signed value from the present components.
	v := float32(intval) + float32(fractval)*(1.0/(1<<5))
	if neg {
		v = -v
	}
	return v, nil
}

// readAngle reads an n-bit quantized angle spanning the full 360 degrees.
func (r *packetReader) readAngle(n uint8) (float32, error) {
	v, err := r.readBits(n)
	if err != nil {
		return 0, err
	}
	return float32(v) * 360.0 / float32(uint32(1)<<n), nil
}

// readNormal reads an 11-bit quantized unit-normal component with a sign bit.
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

// read3BitNormal reads a Valve compressed 3-component unit normal: presence
// flags for x and y, then a derived z whose sign is transmitted directly.
func (r *packetReader) read3BitNormal() ([3]float32, error) {
	var ret [3]float32

	// Read presence flags and any transmitted components.
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

	// Derive z from the unit-length constraint and apply the transmitted sign.
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
