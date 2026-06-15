package s2replay

import "errors"

// errBadMagic indicates the input does not start with the PBDEMS2 header.
var errBadMagic = errors.New("s2replay: not a PBDEMS2 demo (bad magic header)")

// errInvalidVarint indicates a malformed varint in the outer demo stream.
var errInvalidVarint = errors.New("s2replay: invalid varint in demo stream")

// errShortRead indicates a length-delimited run ran past the end of the buffer.
var errShortRead = errors.New("s2replay: short read in demo stream")

// errBitReadOverflow indicates a packet bitstream read past its payload.
var errBitReadOverflow = errors.New("s2replay: packet bitstream overflow")

// errNegativePacketSize indicates an inner packet message declared a bad size.
var errNegativePacketSize = errors.New("s2replay: negative packet message size")

// errUnknownEntityClass indicates packet entities referenced a missing class.
var errUnknownEntityClass = errors.New("s2replay: packet entity referenced unknown class")

// errUnknownEntity indicates packet entities referenced a missing entity.
var errUnknownEntity = errors.New("s2replay: packet entity referenced unknown entity")

// errUnknownFieldPath indicates an entity update used an undecodable field path.
var errUnknownFieldPath = errors.New("s2replay: packet entity referenced unknown field path")

// errUnknownStringTable indicates a string-table update referenced a missing table.
var errUnknownStringTable = errors.New("s2replay: string-table update referenced unknown table")
