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
```

The parser API and CLI subcommands land as the milestones below complete.

## License

MIT. See [LICENSE](LICENSE).
