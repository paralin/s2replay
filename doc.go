// Package s2replay parses Source 2 PBDEMS2 replays and emits a clean,
// attributed event stream for downstream analysis.
//
// The first release targets Deadlock (Citadel) only. The parser decodes the
// demo container, network and Citadel user messages, the entity framework, and
// the active-modifier table, then exposes damage, modifier, purchase, and
// entity-sample events keyed by entity index and game time.
package s2replay

// Version is the current s2replay release identifier.
const Version = "0.0.0-dev"
