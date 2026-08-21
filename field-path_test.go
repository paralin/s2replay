package s2replay

import "testing"

func TestFieldPathPopKeepsRoot(t *testing.T) {
	fp := fieldPath{path: [fieldPathMaxDepth]int{7}, last: 0}

	fp.pop(1)

	if fp.last != 0 {
		t.Fatalf("pop removed root: last=%d", fp.last)
	}
	if fp.path[0] != 7 {
		t.Fatalf("pop changed root: got %d", fp.path[0])
	}
}

func TestFieldPathIncrementPenultimateAtRoot(t *testing.T) {
	fp := fieldPath{path: [fieldPathMaxDepth]int{7}, last: 0}

	fp.incrementPenultimate()

	if fp.last != 0 {
		t.Fatalf("increment changed depth: last=%d", fp.last)
	}
	if fp.path[0] != 8 {
		t.Fatalf("root increment = %d, want 8", fp.path[0])
	}
}

func TestFieldPathIncrementPenultimateNested(t *testing.T) {
	fp := fieldPath{path: [fieldPathMaxDepth]int{7, 3}, last: 1}

	fp.incrementPenultimate()

	if fp.path[0] != 8 {
		t.Fatalf("parent increment = %d, want 8", fp.path[0])
	}
	if fp.path[1] != 3 {
		t.Fatalf("leaf changed = %d, want 3", fp.path[1])
	}
}
