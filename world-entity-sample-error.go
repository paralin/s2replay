package s2replay

// WorldEntitySampleError reports malformed direct entity evidence.
type WorldEntitySampleError struct {
	// EntityID identifies the replay entity with malformed evidence.
	EntityID int32
	// EntitySerial distinguishes generations that reuse EntityID.
	EntitySerial int32
	// Field names the non-finite sample field.
	Field string
}

// Error identifies the malformed field; entity identity remains in the record.
func (e *WorldEntitySampleError) Error() string {
	return "s2replay: non-finite world entity sample field " + e.Field
}

// _ is a type assertion
var _ error = (*WorldEntitySampleError)(nil)
