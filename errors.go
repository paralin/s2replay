package s2replay

import "errors"

// errBadMagic indicates the input does not start with the PBDEMS2 header.
var errBadMagic = errors.New("s2replay: not a PBDEMS2 demo (bad magic header)")

// errInvalidVarint indicates a malformed varint in the outer demo stream.
var errInvalidVarint = errors.New("s2replay: invalid varint in demo stream")

// errShortRead indicates a length-delimited run ran past the end of the buffer.
var errShortRead = errors.New("s2replay: short read in demo stream")
