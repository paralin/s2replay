package s2replay

import (
	"strconv"
	"strings"
)

var fieldPathTree = newFieldPathTree()

type fieldPath struct {
	path [7]int
	last int
	done bool
}

type fieldPathOp struct {
	weight int
	fn     func(r *packetReader, fp *fieldPath) error
}

var fieldPathOps = []fieldPathOp{
	{36271, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last]++; return nil }},
	{10334, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last] += 2; return nil }},
	{1375, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last] += 3; return nil }},
	{646, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last] += 4; return nil }},
	{4128, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		fp.path[fp.last] += v + 5
		return err
	}},
	{35, func(_ *packetReader, fp *fieldPath) error { fp.push(0); return nil }},
	{3, func(r *packetReader, fp *fieldPath) error { v, err := r.readUBitVarFieldPath(); fp.push(v); return err }},
	{521, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last]++; fp.push(0); return nil }},
	{2942, func(r *packetReader, fp *fieldPath) error {
		fp.path[fp.last]++
		v, err := r.readUBitVarFieldPath()
		fp.push(v)
		return err
	}},
	{560, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		fp.path[fp.last] += v
		fp.push(0)
		return err
	}},
	{471, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		if err != nil {
			return err
		}
		fp.path[fp.last] += v + 2
		v, err = r.readUBitVarFieldPath()
		fp.push(v + 1)
		return err
	}},
	{10530, func(r *packetReader, fp *fieldPath) error {
		left, err := r.readBits(3)
		if err != nil {
			return err
		}
		right, err := r.readBits(3)
		fp.path[fp.last] += int(left) + 2
		fp.push(int(right) + 1)
		return err
	}},
	{251, func(r *packetReader, fp *fieldPath) error {
		left, err := r.readBits(4)
		if err != nil {
			return err
		}
		right, err := r.readBits(4)
		fp.path[fp.last] += int(left) + 2
		fp.push(int(right) + 1)
		return err
	}},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushRead(r, 2, false, 0) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushPacked(r, 2, 5, false, 0) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushRead(r, 3, false, 0) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushPacked(r, 3, 5, false, 0) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushRead(r, 2, true, 1) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushPacked(r, 2, 5, true, 1) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushRead(r, 3, true, 1) }},
	{0, func(r *packetReader, fp *fieldPath) error { return fp.pushPacked(r, 3, 5, true, 1) }},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVar()
		fp.path[fp.last] += int(v) + 2
		return fp.pushRead(r, 2, false, 0, err)
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVar()
		fp.path[fp.last] += int(v) + 2
		return fp.pushPacked(r, 2, 5, false, 0, err)
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVar()
		fp.path[fp.last] += int(v) + 2
		return fp.pushRead(r, 3, false, 0, err)
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVar()
		fp.path[fp.last] += int(v) + 2
		return fp.pushPacked(r, 3, 5, false, 0, err)
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		n, err := r.readUBitVar()
		if err != nil {
			return err
		}
		v, err := r.readUBitVar()
		if err != nil {
			return err
		}
		fp.path[fp.last] += int(v)
		for i := 0; i < int(n); i++ {
			v, err := r.readUBitVarFieldPath()
			if err != nil {
				return err
			}
			fp.push(v)
		}
		return nil
	}},
	{310, func(r *packetReader, fp *fieldPath) error {
		for i := 0; i <= fp.last; i++ {
			v, err := r.readBool()
			if err != nil {
				return err
			}
			if v {
				d, err := r.readVarint32()
				if err != nil {
					return err
				}
				fp.path[i] += int(d) + 1
			}
		}
		n, err := r.readUBitVar()
		if err != nil {
			return err
		}
		for i := 0; i < int(n); i++ {
			v, err := r.readUBitVarFieldPath()
			if err != nil {
				return err
			}
			fp.push(v)
		}
		return nil
	}},
	{2, func(_ *packetReader, fp *fieldPath) error { fp.pop(1); fp.path[fp.last]++; return nil }},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		fp.pop(1)
		fp.path[fp.last] += v + 1
		return err
	}},
	{1837, func(_ *packetReader, fp *fieldPath) error { fp.pop(fp.last); fp.path[0]++; return nil }},
	{149, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		fp.pop(fp.last)
		fp.path[0] += v + 1
		return err
	}},
	{300, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readBits(3)
		fp.pop(fp.last)
		fp.path[0] += int(v) + 1
		return err
	}},
	{634, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readBits(6)
		fp.pop(fp.last)
		fp.path[0] += int(v) + 1
		return err
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		v, err := r.readUBitVarFieldPath()
		fp.pop(v)
		fp.path[fp.last]++
		return err
	}},
	{0, func(r *packetReader, fp *fieldPath) error {
		n, err := r.readUBitVarFieldPath()
		if err != nil {
			return err
		}
		d, err := r.readVarint32()
		fp.pop(n)
		fp.path[fp.last] += int(d)
		return err
	}},
	{1, func(r *packetReader, fp *fieldPath) error {
		n, err := r.readUBitVarFieldPath()
		if err != nil {
			return err
		}
		fp.pop(n)
		return fp.nonTopo(r, 0)
	}},
	{76, func(r *packetReader, fp *fieldPath) error { return fp.nonTopo(r, 0) }},
	{271, func(_ *packetReader, fp *fieldPath) error { fp.path[fp.last-1]++; return nil }},
	{99, func(r *packetReader, fp *fieldPath) error { return fp.nonTopo(r, 4) }},
	{25474, func(_ *packetReader, fp *fieldPath) error { fp.done = true; return nil }},
}

func (fp *fieldPath) push(v int) {
	fp.last++
	fp.path[fp.last] = v
}

func (fp fieldPath) String() string {
	if fp.last < 0 {
		return ""
	}
	var s strings.Builder
	s.WriteString(strconv.Itoa(fp.path[0]))
	for i := 1; i <= fp.last; i++ {
		s.WriteString("." + strconv.Itoa(fp.path[i]))
	}
	return s.String()
}

func (fp *fieldPath) pop(n int) {
	for range n {
		fp.path[fp.last] = 0
		fp.last--
	}
}

func (fp *fieldPath) pushRead(r *packetReader, count int, inc bool, delta int, prev ...error) error {
	if len(prev) > 0 && prev[0] != nil {
		return prev[0]
	}
	if inc {
		fp.path[fp.last] += delta
	}
	for range count {
		v, err := r.readUBitVarFieldPath()
		if err != nil {
			return err
		}
		fp.push(v)
	}
	return nil
}

func (fp *fieldPath) pushPacked(r *packetReader, count int, bits uint8, inc bool, delta int, prev ...error) error {
	if len(prev) > 0 && prev[0] != nil {
		return prev[0]
	}
	if inc {
		fp.path[fp.last] += delta
	}
	for range count {
		v, err := r.readBits(bits)
		if err != nil {
			return err
		}
		fp.push(int(v))
	}
	return nil
}

func (fp *fieldPath) nonTopo(r *packetReader, bits uint8) error {
	for i := 0; i <= fp.last; i++ {
		ok, err := r.readBool()
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		var delta int
		if bits == 0 {
			v, err := r.readVarint32()
			if err != nil {
				return err
			}
			delta = int(v)
		} else {
			v, err := r.readBits(bits)
			if err != nil {
				return err
			}
			delta = int(v) - 7
		}
		fp.path[i] += delta
	}
	return nil
}

func readFieldPaths(r *packetReader) ([]fieldPath, error) {
	fp := fieldPath{path: [7]int{-1, 0, 0, 0, 0, 0, 0}}
	node := fieldPathTree
	paths := make([]fieldPath, 0, 8)
	for !fp.done {
		bit, err := r.readBits(1)
		if err != nil {
			return nil, err
		}
		if bit == 1 {
			node = node.right()
		} else {
			node = node.left()
		}
		if node.isLeaf() {
			if err := fieldPathOps[node.value()].fn(r, &fp); err != nil {
				return nil, err
			}
			if !fp.done {
				paths = append(paths, fp)
			}
			node = fieldPathTree
		}
	}
	return paths, nil
}

func newFieldPathTree() huffmanTree {
	freqs := make([]int, len(fieldPathOps))
	for i, op := range fieldPathOps {
		freqs[i] = op.weight
	}
	return buildHuffmanTree(freqs)
}
