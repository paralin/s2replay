# s2replay

A modern Source 2 (PBDEMS2) replay parser. The first release targets Deadlock
(Citadel) only and is tuned to emit a clean, attributed event stream for
downstream analysis.

## Status

Early development. The parser is being built up in measurable slices: demo
container and clock, network/Citadel message dispatch, the entity framework,
and the active-modifier table.

## What it emits

s2replay decodes a `.dem` replay into a stream of events keyed by entity index
and game time:

- Damage events with full context (attacker, victim, inflictor, ability,
  pre/post health and shield, absorbed, crit, effectiveness).
- Modifier add/remove/refresh with duration, caster, parent entity, and stack
  count.
- Item purchase events tying an item to its owner at a game time.
- Per-tick entity samples for hero health, shield, and position.

These streams let a consumer cluster events into fights and build per-item
behavior profiles.

## Usage

```
s2replay version
s2replay parse <demo.dem>
```

`parse` opens a PBDEMS2 demo, walks its outer command stream, and prints the
file header plus a monotonic tick / game-time stream. Further subcommands and
the event API land as the milestones below complete.

## Protocol generation

The Deadlock protocol Go package under `protocol/` is generated, never
hand-edited. `generator/update_protos.bash` copies a minimal Deadlock allow-list
from the pinned `generator/Protobufs` submodule, flattens it into one
`protocol` package, and strips proto2 extensions, custom options, and the heavy
Steam-GC / descriptor imports the wire decode path does not need. Generation
runs through the [aperturerobotics/common](https://github.com/aperturerobotics/common)
(`aptre`) reflect-free `protobuf-go-lite` pipeline with Go outputs only:

```
make gen
```

This is reproducible: a clean re-run produces no diff. Do not edit `*.pb.go`.

## Development

```
make lint
make test
```

## License

MIT. See [LICENSE](LICENSE).
