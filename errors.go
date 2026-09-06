package s2replay

import "errors"

var (
	// errBadMagic indicates the input does not start with the PBDEMS2 header.
	errBadMagic = errors.New("s2replay: not a PBDEMS2 demo (bad magic header)")

	// errInvalidVarint indicates a malformed varint in the outer demo stream.
	errInvalidVarint = errors.New("s2replay: invalid varint in demo stream")

	// errShortRead indicates a length-delimited run ran past the end of the buffer.
	errShortRead = errors.New("s2replay: short read in demo stream")

	// errBitReadOverflow indicates a packet bitstream read past its payload.
	errBitReadOverflow = errors.New("s2replay: packet bitstream overflow")

	// errNegativePacketSize indicates an inner packet message declared a bad size.
	errNegativePacketSize = errors.New("s2replay: negative packet message size")

	// errUnknownEntityClass indicates packet entities referenced a missing class.
	errUnknownEntityClass = errors.New("s2replay: packet entity referenced unknown class")

	// errUnknownEntity indicates packet entities referenced a missing entity.
	errUnknownEntity = errors.New("s2replay: packet entity referenced unknown entity")

	// errUnknownFieldPath indicates an entity update used an undecodable field path.
	errUnknownFieldPath = errors.New("s2replay: packet entity referenced unknown field path")

	// errUnknownStringTable indicates a string-table update referenced a missing table.
	errUnknownStringTable = errors.New("s2replay: string-table update referenced unknown table")

	// errInvalidWorldSnapshotTick rejects the pre-game sentinel as a timecode.
	errInvalidWorldSnapshotTick = errors.New("s2replay: invalid world snapshot tick")

	// errWorldSnapshotPastTick rejects a request older than the parser position.
	errWorldSnapshotPastTick = errors.New("s2replay: world snapshot tick is behind parser position")

	// errInvalidAbilitySlot rejects values outside the engine's uint16 enum domain.
	errInvalidAbilitySlot = errors.New("s2replay: invalid ability slot enum")

	// errInvalidStringTableUpdateCount indicates a negative string-table update count.
	errInvalidStringTableUpdateCount = errors.New("s2replay: negative string-table update count")

	// errStringTableUpdateCountTooLarge indicates an implausible string-table update count.
	errStringTableUpdateCountTooLarge = errors.New("s2replay: string-table update count too large")

	// errStringTableIndexTooLarge indicates an explicit entry index exceeds the Source limit.
	errStringTableIndexTooLarge = errors.New("s2replay: string-table index too large")

	// errInvalidStringTableUserDataSize indicates a negative fixed user-data bit count.
	errInvalidStringTableUserDataSize = errors.New("s2replay: negative string-table user-data size")

	// errStringTableUserDataTooLarge indicates user data exceeds the Source string-table limit.
	errStringTableUserDataTooLarge = errors.New("s2replay: string-table user data too large")

	// errStringTableKeyTooLarge indicates a key exceeds the Source network-string limit.
	errStringTableKeyTooLarge = errors.New("s2replay: string-table key too large")

	// errStringTableDataTooLarge indicates a compressed table expands beyond the parser limit.
	errStringTableDataTooLarge = errors.New("s2replay: string-table data too large")
)
