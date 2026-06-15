package s2replay

import "testing"

func TestQuantizedFloatPlaybackRateClearsEncodeZero(t *testing.T) {
	bits := int32(8)
	flags := int32(qffRoundDown | qffEncodeZero)
	low := float32(-4)
	high := float32(12)

	q := newQuantizedFloatDecoder(&bits, &flags, &low, &high)
	if q.flags != 0 {
		t.Fatalf("flags = %d, want 0; low=%v high=%v highLowMul=%v decMul=%v quantizeZero=%v", q.flags, q.low, q.high, q.highLowMul, q.decMul, q.quantize(0))
	}
}
