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

// errInvalidStringTableUpdateCount indicates a negative string-table update count.
var errInvalidStringTableUpdateCount = errors.New("s2replay: negative string-table update count")

// errStringTableUpdateCountTooLarge indicates an implausible string-table update count.
var errStringTableUpdateCountTooLarge = errors.New("s2replay: string-table update count too large")

// errStringTableIndexTooLarge indicates an explicit entry index exceeds the Source limit.
var errStringTableIndexTooLarge = errors.New("s2replay: string-table index too large")

// errInvalidStringTableUserDataSize indicates a negative fixed user-data bit count.
var errInvalidStringTableUserDataSize = errors.New("s2replay: negative string-table user-data size")

// errStringTableUserDataTooLarge indicates user data exceeds the Source string-table limit.
var errStringTableUserDataTooLarge = errors.New("s2replay: string-table user data too large")

// errStringTableKeyTooLarge indicates a key exceeds the Source network-string limit.
var errStringTableKeyTooLarge = errors.New("s2replay: string-table key too large")

// errStringTableDataTooLarge indicates a compressed table expands beyond the parser limit.
var errStringTableDataTooLarge = errors.New("s2replay: string-table data too large")

// errModifierRefreshWithoutAdd indicates a refresh arrived before an active entry.
var errModifierRefreshWithoutAdd = errors.New("s2replay: modifier refresh without prior add")

// errModifierRemoveWithoutAdd indicates a removal arrived before an active entry.
var errModifierRemoveWithoutAdd = errors.New("s2replay: modifier remove without prior add")
