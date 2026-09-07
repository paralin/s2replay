package s2replay

import (
	"context"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestParserCancellationStopsConsumers(t *testing.T) {
	// Cancellation must reach both event and world-snapshot command consumers.
	demo := buildDemo(t, []Command{{Kind: protocol.EDemoCommands_DEM_Packet, Tick: 100}})
	for _, consumer := range []string{"event", "snapshot"} {
		t.Run(consumer, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			parser, err := NewParserWithContext(ctx, demo)
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			switch consumer {
			case "event":
				_, err = parser.NextEvent()
			case "snapshot":
				_, err = parser.WorldEntitySnapshot(100)
			}
			if err != context.Canceled {
				t.Fatalf("consumer ignored cancellation: %v", err)
			}
			if parser.Clock().Tick() != 0 {
				t.Fatal("canceled consumer advanced the parser")
			}
		})
	}
}
