package s2replay

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
