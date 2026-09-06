package s2replay

// WorldSnapshotError reports that a requested snapshot tick was not observed.
type WorldSnapshotError struct {
	// RequestedTick is the snapshot tick requested by the caller.
	RequestedTick uint32
	// FinalTick is the last tick observed before the replay ended.
	FinalTick uint32
}

// Error describes the missing snapshot without changing its structured evidence.
func (e *WorldSnapshotError) Error() string {
	return "s2replay: world snapshot tick not observed"
}

// _ is a type assertion
var _ error = (*WorldSnapshotError)(nil)
