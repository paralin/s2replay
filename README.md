# s2replay

A modern Source 2 (PBDEMS2) replay parser. The first release targets Deadlock
(Citadel) only and is tuned to emit a clean, attributed event stream for
downstream analysis.

## Status

The Deadlock parser is usable end to end. It decodes PBDEMS2 containers,
dispatches generated Source 2 and Citadel messages, tracks entity and modifier
state, emits attributed JSONL events, and derives replay-local analysis data.
The public event stream remains at schema version 1. The separate analysis
output is at schema version 2 and may evolve as more replay evidence becomes
available.

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
s2replay analyze --format json [--combat-gap seconds] [--combat-events damage,modifier] <demo.dem>
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

## Analysis primitives

The Go package `github.com/paralin/s2replay/analysis` derives replay-local
timelines from the typed event stream:

- `analysis.Build(events)` returns inventory ownership intervals, entity health
  samples, modifier lifecycle intervals, and quality counters.
- `analysis.BuildCombatWindows(events, options)` groups caller-selected events
  with caller-owned timing policy.

These primitives use only replay events. They do not fetch Deadlock API data,
read static item catalogs, score item behavior, or assign item-profile policy.
Modifier source ids remain raw replay source identity; they are not promoted to
parser-owned item ids without fixture-backed proof. Entity samples in analysis
output include item context derived from parser-owned inventory state without
changing the default `emit --format jsonl` event stream.

`analyze --format json` is the process boundary for non-Go consumers. It emits a
separate analysis schema:

- `schema_version`: analysis schema version. Current value: `2`.
- `analysis`: the Go analysis result.
- `combat_windows`: present only when `--combat-gap` is non-negative.

Combat windows have no parser default fight policy. Callers must pass
`--combat-gap` and may restrict selected events with `--combat-events`.
Each combat window reports the selected event count, per-type counts, first and
last selected event indexes, player slots observed in the window, primary
entity ids, damage attacker/victim ids, and modifier parent ids. Player slots
include damage victims when an earlier entity sample established the victim
entity's slot. The default `emit --format jsonl` event stream is unchanged by
analysis output.

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
