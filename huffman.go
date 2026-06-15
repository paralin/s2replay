package s2replay

type huffmanTree interface {
	isLeaf() bool
	value() int
	left() huffmanTree
	right() huffmanTree
	weight() int
	order() int
}

type huffmanLeaf struct {
	w int
	v int
}

func (l huffmanLeaf) isLeaf() bool       { return true }
func (l huffmanLeaf) value() int         { return l.v }
func (l huffmanLeaf) left() huffmanTree  { return nil }
func (l huffmanLeaf) right() huffmanTree { return nil }
func (l huffmanLeaf) weight() int        { return l.w }
func (l huffmanLeaf) order() int         { return l.v }

type huffmanNode struct {
	w int
	o int
	l huffmanTree
	r huffmanTree
}

func (n huffmanNode) isLeaf() bool       { return false }
func (n huffmanNode) value() int         { return -1 }
func (n huffmanNode) left() huffmanTree  { return n.l }
func (n huffmanNode) right() huffmanTree { return n.r }
func (n huffmanNode) weight() int        { return n.w }
func (n huffmanNode) order() int         { return n.o }

func buildHuffmanTree(freqs []int) huffmanTree {
	nodes := make([]huffmanTree, 0, len(freqs))
	for i, freq := range freqs {
		if freq == 0 {
			freq = 1
		}
		nodes = append(nodes, huffmanLeaf{w: freq, v: i})
	}
	order := len(freqs)
	for len(nodes) > 1 {
		a := minHuffman(nodes)
		left := nodes[a]
		nodes = append(nodes[:a], nodes[a+1:]...)
		b := minHuffman(nodes)
		right := nodes[b]
		nodes = append(nodes[:b], nodes[b+1:]...)
		nodes = append(nodes, huffmanNode{w: left.weight() + right.weight(), o: order, l: left, r: right})
		order++
	}
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func minHuffman(nodes []huffmanTree) int {
	min := 0
	for i := 1; i < len(nodes); i++ {
		if nodes[i].weight() < nodes[min].weight() || nodes[i].weight() == nodes[min].weight() && nodes[i].order() > nodes[min].order() {
			min = i
		}
	}
	return min
}
