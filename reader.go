package s2replay

import "encoding/binary"

// reader is a byte cursor over the demo container's outer stream of
// (command, tick, size, payload) records. That stream is byte-aligned, so it
// needs only unsigned varints and length-delimited byte runs. Inner packet
// bit-reading is a separate concern owned by a later decode layer.
type reader struct {
	buf []byte
	pos int
}

// remaining reports how many bytes are left to read.
func (r *reader) remaining() int { return len(r.buf) - r.pos }

// readUvarint reads a base-128 unsigned varint and advances the cursor.
func (r *reader) readUvarint() (uint64, error) {
	v, n := binary.Uvarint(r.buf[r.pos:])
	if n <= 0 {
		return 0, errInvalidVarint
	}
	r.pos += n
	return v, nil
}

// readBytes returns the next n bytes as a sub-slice of the backing buffer and
// advances the cursor. The result aliases the input; callers that retain it
// past the next demo must copy.
func (r *reader) readBytes(n int) ([]byte, error) {
	// Compare against the remaining length rather than r.pos+n so a huge
	// attacker-controlled size varint cannot overflow the addition past the
	// guard and panic on the slice instead of returning errShortRead.
	if n < 0 || n > r.remaining() {
		return nil, errShortRead
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}
