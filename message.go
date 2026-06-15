package s2replay

import "github.com/paralin/s2replay/protocol"

type decodedProto interface {
	UnmarshalVT([]byte) error
}

type decodedMessage struct {
	kind int32
	name string
	msg  decodedProto
}

// Message is a decoded packet or user message with demo tick and game time.
type Message struct {
	Kind     int32
	Name     string
	Tick     uint32
	GameTime float64
	Payload  decodedProto
	err      error
}

// DamageEvent is the Phase 3 product projection of a Deadlock damage user
// message. Later phases add entity class and item attribution around it.
type DamageEvent struct {
	Tick                     uint32  `json:"tick"`
	GameTime                 float64 `json:"game_time"`
	Damage                   int32   `json:"damage"`
	PreDamage                float32 `json:"pre_damage"`
	VictimHealthNew          int32   `json:"victim_health_new"`
	VictimHealthMax          int32   `json:"victim_health_max"`
	DamageAbsorbed           float32 `json:"damage_absorbed"`
	Effectiveness            float32 `json:"effectiveness"`
	CritDamage               float32 `json:"crit_damage"`
	Hits                     int32   `json:"hits"`
	Attacker                 int32   `json:"attacker"`
	Victim                   int32   `json:"victim"`
	Inflictor                int32   `json:"inflictor"`
	AbilityEntity            int32   `json:"ability_entity"`
	AbilityID                uint32  `json:"ability_id"`
	DamageType               int32   `json:"damage_type"`
	CitadelDamageType        int32   `json:"citadel_damage_type"`
	AttackingObject          int32   `json:"attacking_object"`
	VictimShieldNew          int32   `json:"victim_shield_new"`
	VictimShieldMax          int32   `json:"victim_shield_max"`
	HealthLost               int32   `json:"health_lost"`
	HitgroupID               int32   `json:"hitgroup_id"`
	IsSecondaryStat          bool    `json:"is_secondary_stat"`
	OriginX                  float32 `json:"origin_x"`
	OriginY                  float32 `json:"origin_y"`
	OriginZ                  float32 `json:"origin_z"`
	DamageDirectionX         float32 `json:"damage_direction_x"`
	DamageDirectionY         float32 `json:"damage_direction_y"`
	DamageDirectionZ         float32 `json:"damage_direction_z"`
	ServerTick               int32   `json:"server_tick"`
	Flags                    uint64  `json:"flags"`
	AttackerClass            uint32  `json:"attacker_class"`
	VictimClass              uint32  `json:"victim_class"`
	PreDamageDeprecated      int32   `json:"pre_damage_deprecated"`
	DamageAbsorbedDeprecated int32   `json:"damage_absorbed_deprecated"`
}

// DamageEvent returns the full-context damage projection for this message.
func (m *Message) DamageEvent() (DamageEvent, bool) {
	d, ok := m.Payload.(*protocol.CCitadelUserMessage_Damage)
	if !ok {
		return DamageEvent{}, false
	}
	ev := DamageEvent{
		Tick:                     m.Tick,
		GameTime:                 m.GameTime,
		Damage:                   d.GetDamage(),
		PreDamage:                d.GetPreDamage(),
		VictimHealthNew:          d.GetVictimHealthNew(),
		VictimHealthMax:          d.GetVictimHealthMax(),
		DamageAbsorbed:           d.GetDamageAbsorbed(),
		Effectiveness:            d.GetEffectiveness(),
		CritDamage:               d.GetCritDamage(),
		Hits:                     d.GetHits(),
		Attacker:                 d.GetEntindexAttacker(),
		Victim:                   d.GetEntindexVictim(),
		Inflictor:                d.GetEntindexInflictor(),
		AbilityEntity:            d.GetEntindexAbility(),
		AbilityID:                d.GetAbilityId(),
		DamageType:               d.GetType(),
		CitadelDamageType:        d.GetCitadelType(),
		AttackingObject:          d.GetEntindexAttackingObject(),
		VictimShieldNew:          d.GetVictimShieldNew(),
		VictimShieldMax:          d.GetVictimShieldMax(),
		HealthLost:               d.GetHealthLost(),
		HitgroupID:               d.GetHitgroupId(),
		IsSecondaryStat:          d.GetIsSecondaryStat(),
		ServerTick:               d.GetServerTick(),
		Flags:                    d.GetFlags(),
		AttackerClass:            d.GetAttackerClass(),
		VictimClass:              d.GetVictimClass(),
		PreDamageDeprecated:      d.GetPreDamageDeprecated(),
		DamageAbsorbedDeprecated: d.GetDamageAbsorbedDeprecated(),
	}
	if origin := d.GetOrigin(); origin != nil {
		ev.OriginX = origin.GetX()
		ev.OriginY = origin.GetY()
		ev.OriginZ = origin.GetZ()
	}
	if dir := d.GetDamageDirection(); dir != nil {
		ev.DamageDirectionX = dir.GetX()
		ev.DamageDirectionY = dir.GetY()
		ev.DamageDirectionZ = dir.GetZ()
	}
	return ev, true
}
