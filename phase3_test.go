package s2replay

import (
	"io"
	"testing"
)

func TestNextDamageReportsEOF(t *testing.T) {
	p, err := NewParser(buildDemo(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.NextDamage(); err != io.EOF {
		t.Fatalf("empty demo: want io.EOF, got %v", err)
	}
}
