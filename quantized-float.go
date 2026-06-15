package s2replay

import "math"

const (
	qffRoundDown      uint32 = 1 << 0
	qffRoundUp        uint32 = 1 << 1
	qffEncodeZero     uint32 = 1 << 2
	qffEncodeIntegers uint32 = 1 << 3

	quantizedFloatZeroEpsilon = 1e-6
)

type quantizedFloatDecoder struct {
	low        float32
	high       float32
	highLowMul float32
	decMul     float32
	bitCount   uint32
	flags      uint32
	noScale    bool
}

func newQuantizedFloatDecoder(bitCount, flags *int32, lowValue, highValue *float32) *quantizedFloatDecoder {
	q := &quantizedFloatDecoder{}
	if bitCount == nil || *bitCount == 0 || *bitCount >= 32 {
		q.noScale = true
		q.bitCount = 32
		return q
	}
	q.bitCount = uint32(*bitCount)
	q.low = 0
	if lowValue != nil {
		q.low = *lowValue
	}
	q.high = 1
	if highValue != nil {
		q.high = *highValue
	}
	if flags != nil {
		q.flags = uint32(*flags)
	}
	q.validateFlags()
	steps := uint32(1 << q.bitCount)
	if q.flags&qffRoundDown != 0 {
		offset := (q.high - q.low) / float32(steps)
		q.high -= offset
	} else if q.flags&qffRoundUp != 0 {
		offset := (q.high - q.low) / float32(steps)
		q.low += offset
	}
	if q.flags&qffEncodeIntegers != 0 {
		delta := q.high + q.low
		if delta < 1 {
			delta = 1
		}
		rng := uint32(1 << uint(math.Ceil(math.Log2(float64(delta)))))
		for (uint32(1) << q.bitCount) <= rng {
			q.bitCount++
		}
		steps = 1 << q.bitCount
		offset := float32(rng) / float32(steps)
		q.high = q.low + float32(rng) - offset
	}
	q.assignMultipliers(steps)
	if q.flags&qffRoundDown != 0 && q.quantize(q.low) == q.low {
		q.flags &^= qffRoundDown
	}
	if q.flags&qffRoundUp != 0 && q.quantize(q.high) == q.high {
		q.flags &^= qffRoundUp
	}
	if q.flags&qffEncodeZero != 0 && q.quantize(0) == 0 {
		q.flags &^= qffEncodeZero
	}
	return q
}

func (q *quantizedFloatDecoder) validateFlags() {
	if q.flags == 0 {
		return
	}
	if (q.low == 0 && q.flags&qffRoundDown != 0) || (q.high == 0 && q.flags&qffRoundUp != 0) {
		q.flags &^= qffEncodeZero
	}
	if q.low == 0 && q.flags&qffEncodeZero != 0 {
		q.flags |= qffRoundUp
		q.flags &^= qffEncodeZero
	}
	if q.high == 0 && q.flags&qffEncodeZero != 0 {
		q.flags |= qffRoundDown
		q.flags &^= qffEncodeZero
	}
	if q.low > 0 || q.high < 0 {
		q.flags &^= qffEncodeZero
	}
	if q.flags&qffEncodeIntegers != 0 {
		q.flags &^= qffRoundUp | qffRoundDown | qffEncodeZero
	}
}

func (q *quantizedFloatDecoder) assignMultipliers(steps uint32) {
	rng := q.high - q.low
	high := uint32(1<<q.bitCount) - 1
	if q.bitCount == 32 {
		high = 0xfffffffe
	}
	if math.Abs(float64(rng)) <= 0 {
		q.highLowMul = float32(high)
	} else {
		q.highLowMul = float32(high) / rng
	}
	if q.highLowMul*rng > float32(high) || float64(q.highLowMul*rng) > float64(high) {
		for _, mult := range []float32{0.9999, 0.99, 0.9, 0.8, 0.7} {
			q.highLowMul = float32(high) / rng * mult
			if q.highLowMul*rng <= float32(high) && float64(q.highLowMul*rng) <= float64(high) {
				break
			}
		}
	}
	q.decMul = 1 / float32(steps-1)
}

func (q *quantizedFloatDecoder) quantize(val float32) float32 {
	if val < q.low {
		return q.low
	}
	if val > q.high {
		return q.high
	}
	i := uint32((val - q.low) * q.highLowMul)
	scaled := float32((q.high - q.low) * float32(i))
	return snapQuantizedFloatZero(q.low + scaled*q.decMul)
}

func snapQuantizedFloatZero(v float32) float32 {
	if math.Abs(float64(v)) < quantizedFloatZeroEpsilon {
		return 0
	}
	return v
}

func (q *quantizedFloatDecoder) decode(r *packetReader) (float32, error) {
	if q.noScale {
		return r.readFloat32()
	}
	if q.flags&qffRoundDown != 0 {
		ok, err := r.readBool()
		if err != nil || ok {
			return q.low, err
		}
	}
	if q.flags&qffRoundUp != 0 {
		ok, err := r.readBool()
		if err != nil || ok {
			return q.high, err
		}
	}
	if q.flags&qffEncodeZero != 0 {
		ok, err := r.readBool()
		if err != nil || ok {
			return 0, err
		}
	}
	v, err := r.readBits(uint8(q.bitCount))
	if err != nil {
		return 0, err
	}
	scaled := float32((q.high - q.low) * float32(v))
	return q.low + scaled*q.decMul, nil
}
