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
s2replay emit --format jsonl <demo.dem>
```

`parse` opens a PBDEMS2 demo, walks its outer command stream, and prints the
file header plus a monotonic tick / game-time stream.

`emit --format jsonl` writes one JSON event per line to stdout. Each event has
the common keys below:

| key | type | meaning |
| --- | --- | --- |
| `schema_version` | integer | Event schema version. Current value: `1`. |
| `type` | string | One of `damage`, `modifier`, `purchase`, `entity_sample`. |
| `tick` | integer | Demo tick, with pre-game sentinel ticks normalized to `0`. |
| `game_time` | number | Parser game time in seconds. |
| `entity` | integer | Primary entity index, or `-1` when the event is not entity-owned. |
| `player_slot` | integer | Deadlock player slot, or `-1` when not attributed. |
| `owned_items` | integer array | Current item ability IDs for the attributed player. Present on attributed attacker-side events and purchase events. |

Type-specific payloads live under a key matching the event type:

- `damage`: full Deadlock damage context from `CCitadelUserMessage_Damage`.
- `modifier`: modifier lifecycle details from `ActiveModifiers`.
- `purchase`: `player_slot`, `user_id`, `ability_id`, `change`, `sell`,
  `quickbuy`, and `source`.
- `entity_sample`: hero health, shield, and position sample fields.

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
