package s2replay

import (
	"io"
	"os"
	"testing"
)

// BenchmarkParseDemo walks every event of the demo named by S2REPLAY_BENCH_DEMO,
// decoding all entity state on the way. It skips when the variable is unset.
func BenchmarkParseDemo(b *testing.B) {
	path := os.Getenv("S2REPLAY_BENCH_DEMO")
	if path == "" {
		b.Skip("S2REPLAY_BENCH_DEMO is unset")
	}
	demo, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(demo)))
	b.ReportAllocs()
	for b.Loop() {
		p, err := NewParser(demo)
		if err != nil {
			b.Fatal(err)
		}
		for {
			if _, err := p.NextEvent(); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}
