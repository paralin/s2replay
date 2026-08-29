package s2replay

import (
	"strconv"

	"github.com/paralin/s2replay/protocol"
)

const (
	entityHandleMask    uint64 = (1 << 14) - 1
	invalidEntityHandle uint32 = 16777215
)

// Entity is the parser-owned current state for one networked entity.
type Entity struct {
	index  int32
	serial int32
	class  *entityClass
	active bool
	state  *fieldState
	paths  map[string]fieldPath
	misses map[string]bool
}

// EntitySample is the typed Phase 4 projection used by downstream event code.
type EntitySample struct {
	Tick         uint32  `json:"tick"`
	GameTime     float64 `json:"game_time"`
	Entity       int32   `json:"entity"`
	ClassID      int32   `json:"class_id"`
	ClassName    string  `json:"class_name"`
	Health       float32 `json:"health"`
	MaxHealth    float32 `json:"max_health"`
	Shield       float32 `json:"shield"`
	MaxShield    float32 `json:"max_shield"`
	PositionX    float32 `json:"position_x"`
	PositionY    float32 `json:"position_y"`
	PositionZ    float32 `json:"position_z"`
	HeroID       uint32  `json:"hero_id,omitempty"`
	Team         int32   `json:"team,omitempty"`
	HasHealth    bool    `json:"has_health"`
	HasShield    bool    `json:"has_shield"`
	HasPosition  bool    `json:"has_position"`
	Grounded     bool    `json:"grounded"`
	Crouching    bool    `json:"crouching"`
	HasGrounded  bool    `json:"has_grounded,omitempty"`
	HasCrouching bool    `json:"has_crouching,omitempty"`
	HasHeroID    bool    `json:"has_hero_id"`
	HasTeam      bool    `json:"has_team"`
}

// ControllerSample is one periodic snapshot of a player controller entity:
// the live scoreboard the game itself maintains.
type ControllerSample struct {
	Tick               uint32  `json:"tick"`
	GameTime           float64 `json:"game_time"`
	Entity             int32   `json:"entity"`
	ClassID            int32   `json:"class_id"`
	ClassName          string  `json:"class_name"`
	SteamID            uint64  `json:"steam_id"`
	PlayerName         string  `json:"player_name"`
	NetWorth           int32   `json:"net_worth"`
	HeroDamage         int32   `json:"hero_damage"`
	HeroHealing        int32   `json:"hero_healing"`
	CreepGold          int32   `json:"creep_gold"`
	CreepGoldKill      int32   `json:"creep_gold_kill"`
	CreepGoldNeutral   int32   `json:"creep_gold_neutral"`
	CreepGoldAirOrb    int32   `json:"creep_gold_air_orb"`
	CreepGoldGroundOrb int32   `json:"creep_gold_ground_orb"`
	CreepGoldDeny      int32   `json:"creep_gold_deny"`
	CreepGoldSoloBonus int32   `json:"creep_gold_solo_bonus"`
}

// ObjectiveEvent is one map-objective lifecycle observation: mid boss spawn,
// boss damage, boss kill, or rejuvenator pickup.
type ObjectiveEvent struct {
	Kind            string  `json:"kind"`
	ObjectiveTeam   int32   `json:"objective_team"`
	ObjectiveID     int32   `json:"objective_id"`
	KillingTeam     int32   `json:"killing_team"`
	EntityType      int32   `json:"entity_type"`
	BossesRemaining int32   `json:"bosses_remaining"`
	GameTimeF       float32 `json:"game_time_f"`
}

func newEntity(index, serial int32, class *entityClass) *Entity {
	return &Entity{
		index:  index,
		serial: serial,
		class:  class,
		active: true,
		state:  newFieldState(),
		paths:  make(map[string]fieldPath),
		misses: make(map[string]bool),
	}
}

// Index returns the networked entity index.
func (e *Entity) Index() int32 { return e.index }

// ClassID returns the entity class id.
func (e *Entity) ClassID() int32 { return e.class.id }

// ClassName returns the entity class name.
func (e *Entity) ClassName() string { return e.class.name }

// Get returns the current decoded field value for name.
func (e *Entity) Get(name string) any {
	if fp, ok := e.paths[name]; ok {
		return e.state.get(fp)
	}
	if e.misses[name] || e.class == nil {
		return nil
	}
	fp, ok := e.class.pathForName(name)
	if !ok {
		e.misses[name] = true
		return nil
	}
	e.paths[name] = fp
	return e.state.get(fp)
}

// Float32 returns the current field value as a float32 when possible.
func (e *Entity) Float32(name string) (float32, bool) {
	switch v := e.Get(name).(type) {
	case float32:
		return v, true
	case uint32:
		return float32(v), true
	case uint64:
		return float32(v), true
	case int32:
		return float32(v), true
	case uint16:
		return float32(v), true
	case int16:
		return float32(v), true
	case uint8:
		return float32(v), true
	}
	return 0, false
}

// String returns the current field value as a string when present.
func (e *Entity) String(name string) (string, bool) {
	v, ok := e.Get(name).(string)
	return v, ok
}

// UInt32 returns the current field value as a uint32 when it fits.
func (e *Entity) UInt32(name string) (uint32, bool) {
	switch v := e.Get(name).(type) {
	case uint32:
		return v, true
	case int32:
		if v >= 0 {
			return uint32(v), true
		}
	case uint64:
		if v <= uint64(^uint32(0)) {
			return uint32(v), true
		}
	case int64:
		if v >= 0 && v <= int64(^uint32(0)) {
			return uint32(v), true
		}
	case float32:
		if v >= 0 && v <= float32(^uint32(0)) {
			return uint32(v), true
		}
	}
	return 0, false
}

// Int32 returns the current field value as an int32 when it can be represented
// without changing sign.
func (e *Entity) Int32(name string) (int32, bool) {
	switch v := e.Get(name).(type) {
	case int32:
		return v, true
	case uint32:
		if v <= uint32(^uint32(0)>>1) {
			return int32(v), true
		}
	case uint64:
		if v <= uint64(^uint32(0)>>1) {
			return int32(v), true
		}
	}
	return 0, false
}

func (e *Entity) sample(tick uint32, gameTime float64) (EntitySample, bool) {
	s := EntitySample{
		Tick:      tick,
		GameTime:  gameTime,
		Entity:    e.index,
		ClassID:   e.class.id,
		ClassName: e.class.name,
	}
	s.Health, s.HasHealth = firstFloat32(
		e,
		"m_iHealth",
		"m_iCurrentHealth",
		"m_flHealth",
		"m_CCitadelHealthComponent.m_iHealth",
	)
	s.MaxHealth, _ = firstFloat32(
		e,
		"m_iMaxHealth",
		"m_flMaxHealth",
		"m_CCitadelHealthComponent.m_iMaxHealth",
	)
	s.Shield, s.HasShield = firstFloat32(
		e,
		"m_iShield",
		"m_flShield",
		"m_CCitadelHealthComponent.m_iShield",
	)
	s.MaxShield, _ = firstFloat32(
		e,
		"m_iMaxShield",
		"m_flMaxShield",
		"m_CCitadelHealthComponent.m_iMaxShield",
	)
	s.HeroID, s.HasHeroID = e.UInt32("m_CCitadelHeroComponent.m_spawnedHero.m_nHeroID")
	if !s.HasHeroID {
		s.HeroID, s.HasHeroID = e.UInt32("m_CCitadelHeroComponent.m_loadingHero.m_nHeroID")
	}
	s.Team, s.HasTeam = e.Int32("m_iTeamNum")
	if ground, ok := e.UInt32("m_hGroundEntity"); ok {
		s.Grounded = ground != invalidEntityHandle
		s.HasGrounded = true
	}
	if crouching, ok := e.Get("m_pMovementServices.m_bDucked").(bool); ok {
		s.Crouching = crouching
		s.HasCrouching = true
	}
	// Modern flattened serializers nest body origin under the skeleton
	// instance; older replays expose it directly on CBodyComponent.
	bodyOrigin := []string{
		"CBodyComponent.m_skeletonInstance.m_vecOrigin",
		"m_CBodyComponent.m_skeletonInstance.m_vecOrigin",
		"CBodyComponent",
		"m_CBodyComponent",
	}
	cellNames := func(axis string) []string {
		names := make([]string, 0, len(bodyOrigin))
		for _, base := range bodyOrigin {
			names = append(names, base+".m_"+axis)
		}
		return names
	}
	vecNames := func(axis string) []string {
		names := make([]string, 0, len(bodyOrigin))
		for _, base := range bodyOrigin {
			names = append(names, base+".m_vec"+axis)
		}
		return names
	}
	x, okX := firstFloat32(e, cellNames("cellX")...)
	y, okY := firstFloat32(e, cellNames("cellY")...)
	z, okZ := firstFloat32(e, cellNames("cellZ")...)
	vx, vxOK := firstFloat32(e, vecNames("X")...)
	vy, vyOK := firstFloat32(e, vecNames("Y")...)
	vz, vzOK := firstFloat32(e, vecNames("Z")...)
	if okX && okY && okZ && vxOK && vyOK && vzOK {
		s.PositionX = deadlockCoordFromCell(x, vx)
		s.PositionY = deadlockCoordFromCell(y, vy)
		s.PositionZ = deadlockCoordFromCell(z, vz)
		s.HasPosition = true
	}
	return s, s.HasHealth || s.HasShield || s.HasPosition || s.HasGrounded || s.HasCrouching
}

func firstFloat32(e *Entity, names ...string) (float32, bool) {
	for _, name := range names {
		if v, ok := e.Float32(name); ok {
			return v, true
		}
	}
	return 0, false
}

func deadlockCoordFromCell(cell, vec float32) float32 {
	return float32(int32(cell)*512-16384) + vec
}

// FindEntity returns the current entity for index when known.
func (p *Parser) FindEntity(index int32) *Entity {
	return p.entities[index]
}

// FindEntityByHandle returns the current entity for a Source 2 entity handle.
func (p *Parser) FindEntityByHandle(handle uint64) *Entity {
	e := p.FindEntity(int32(handle & entityHandleMask))
	if e == nil || e.serial != int32(handle>>14) {
		return nil
	}
	return e
}

// NextEntitySample returns the next typed hero/entity sample from packet entity
// updates.
func (p *Parser) NextEntitySample() (EntitySample, error) {
	for len(p.pendingSamples) == 0 {
		if _, err := p.NextMessage(); err != nil {
			return EntitySample{}, err
		}
	}
	s := p.pendingSamples[0]
	copy(p.pendingSamples, p.pendingSamples[1:])
	p.pendingSamples = p.pendingSamples[:len(p.pendingSamples)-1]
	return s, nil
}

func (p *Parser) applyPacketEntities(tick uint32, msg *protocol.CSVCMsg_PacketEntities) error {
	buf := msg.GetEntityData()
	if len(buf) == 0 {
		buf = msg.GetSerializedEntities()
	}
	r := newPacketReader(buf)
	index := int32(-1)
	updates := int(msg.GetUpdatedEntries())
	for ; updates > 0; updates-- {
		off, err := r.readUBitVar()
		if err != nil {
			return err
		}
		index += int32(off) + 1
		cmd, err := r.readBits(2)
		if err != nil {
			return err
		}
		if cmd&1 == 0 {
			if cmd&2 != 0 {
				p.entityCreates++
				classID, err := r.readBits(p.classIDBits)
				if err != nil {
					return err
				}
				serial, err := r.readBits(17)
				if err != nil {
					return err
				}
				if _, err := r.readUvarint32(); err != nil {
					return err
				}
				class := p.classesByID[int32(classID)]
				if class == nil {
					return errUnknownEntityClass
				}
				e := newEntity(index, int32(serial), class)
				p.entities[index] = e
				if baseline := p.classBaselines[int32(classID)]; len(baseline) != 0 {
					if err := e.readFields(newPacketReader(baseline)); err != nil {
						return err
					}
				}
				if err := e.readFields(r); err != nil {
					return err
				}
				p.appendEntitySample(tick, e)
				continue
			}
			e := p.entities[index]
			if e == nil {
				return packetEntityError{tick: tick, index: index, command: cmd, err: errUnknownEntity}
			}
			p.entityUpdates++
			if !e.active {
				e.active = true
			}
			if err := e.readFields(r); err != nil {
				return err
			}
			p.appendEntitySample(tick, e)
			continue
		}
		p.entityLeaves++
		e := p.entities[index]
		if e == nil {
			if cmd&2 != 0 {
				p.entityDeletes++
			}
			continue
		}
		e.active = false
		if cmd&2 != 0 {
			p.entityDeletes++
			delete(p.entities, index)
			delete(p.entityPlayerSlots, index)
		}
	}
	return nil
}

func (e *Entity) readFields(r *packetReader) error {
	paths, err := readFieldPaths(r)
	if err != nil {
		return entityDecodeError{entity: e, err: err}
	}
	for _, fp := range paths {
		d := e.class.decoder(fp)
		if d == nil {
			return entityDecodeError{entity: e, path: fp, field: e.class.fieldByPath(fp), rootField: e.class.rootField(fp), fieldName: e.class.fieldName(fp), err: errUnknownFieldPath}
		}
		v, err := d(r)
		if err != nil {
			return entityDecodeError{entity: e, path: fp, field: e.class.fieldByPath(fp), rootField: e.class.rootField(fp), fieldName: e.class.fieldName(fp), err: err}
		}
		e.state.set(fp, v)
	}
	return nil
}

type entityDecodeError struct {
	entity    *Entity
	path      fieldPath
	field     *field
	rootField *field
	fieldName string
	err       error
}

func (e entityDecodeError) Error() string {
	s := e.err.Error()
	if e.entity != nil {
		s += " entity=" + strconv.Itoa(int(e.entity.index))
		if e.entity.class != nil {
			s += " class=" + e.entity.class.name
		}
	}
	if e.path.last >= 0 {
		s += " path=" + e.path.String()
	}
	if e.fieldName != "" {
		s += " field=" + e.fieldName
	}
	if e.field != nil {
		s += " type=" + e.field.varType + " model=" + strconv.Itoa(e.field.model)
	}
	if e.rootField != nil && e.rootField != e.field {
		s += " root_type=" + e.rootField.varType + " root_model=" + strconv.Itoa(e.rootField.model)
		if e.rootField.serializerName != "" {
			s += " root_serializer=" + e.rootField.serializerName
		}
		if e.rootField.serializer != nil {
			s += " root_fields=" + strconv.Itoa(len(e.rootField.serializer.fields))
		}
	}
	return s
}

func (e entityDecodeError) Unwrap() error {
	return e.err
}

type packetEntityError struct {
	tick    uint32
	index   int32
	command uint32
	err     error
}

func (e packetEntityError) Error() string {
	return e.err.Error() +
		" tick=" + strconv.FormatUint(uint64(e.tick), 10) +
		" entity=" + strconv.Itoa(int(e.index)) +
		" command=" + strconv.FormatUint(uint64(e.command), 10)
}

func (e packetEntityError) Unwrap() error {
	return e.err
}

func (p *Parser) appendControllerSample(tick uint32, e *Entity) {
	// One sample per controller per second of game time keeps the stream
	// bounded while still resolving economy curves.
	if t, ok := p.lastControllerSample[e.index]; ok && tick-t < 64 {
		return
	}
	p.lastControllerSample[e.index] = tick
	cs := &ControllerSample{
		Tick:      normalizedTick(tick),
		GameTime:  p.clock.GameTime(),
		Entity:    e.index,
		ClassID:   e.class.id,
		ClassName: e.class.name,
	}
	if v, ok := e.Get("m_steamID").(uint64); ok {
		cs.SteamID = v
	}
	cs.PlayerName, _ = e.String("m_iszPlayerName")
	intFields := []struct {
		name string
		dst  *int32
	}{
		{"m_PlayerDataGlobal.m_iGoldNetWorth", &cs.NetWorth},
		{"m_PlayerDataGlobal.m_iHeroDamage", &cs.HeroDamage},
		{"m_PlayerDataGlobal.m_iHeroHealing", &cs.HeroHealing},
		{"m_PlayerDataGlobal.m_iCreepGold", &cs.CreepGold},
		{"m_PlayerDataGlobal.m_iCreepGoldKill", &cs.CreepGoldKill},
		{"m_PlayerDataGlobal.m_iCreepGoldNeutral", &cs.CreepGoldNeutral},
		{"m_PlayerDataGlobal.m_iCreepGoldAirOrb", &cs.CreepGoldAirOrb},
		{"m_PlayerDataGlobal.m_iCreepGoldGroundOrb", &cs.CreepGoldGroundOrb},
		{"m_PlayerDataGlobal.m_iCreepGoldDeny", &cs.CreepGoldDeny},
		{"m_PlayerDataGlobal.m_iCreepGoldSoloBonus", &cs.CreepGoldSoloBonus},
	}
	for _, f := range intFields {
		v, ok := e.Int32(f.name)
		if ok {
			*f.dst = v
		}
	}
	slot, ok := e.Int32("m_unLobbyPlayerSlot")
	if !ok {
		slot = -1
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:             EventControllerSample,
		Tick:             cs.Tick,
		GameTime:         cs.GameTime,
		Entity:           e.index,
		PlayerSlot:       slot,
		ControllerSample: cs,
	})
}

func isPlayerControllerClass(name string) bool {
	for i := 0; i+len("CitadelPlayerController") <= len(name); i++ {
		if name[i:i+len("CitadelPlayerController")] == "CitadelPlayerController" {
			return true
		}
	}
	return false
}

func (p *Parser) appendEntitySample(tick uint32, e *Entity) {
	if e == nil || e.class == nil || !e.active {
		return
	}
	p.updateEntityPlayerSlot(e)
	if isPlayerControllerClass(e.class.name) {
		p.appendControllerSample(tick, e)
		return
	}
	if stringsContains(e.class.name, "Ability") {
		p.appendAbilityChargeEvent(tick, e)
		return
	}
	if !isLikelyHeroClass(e.class.name) {
		return
	}
	if sample, ok := e.sample(tick, p.clock.GameTime()); ok {
		p.pendingSamples = append(p.pendingSamples, sample)
		slot, ok := p.entityPlayerSlots[sample.Entity]
		if !ok {
			slot = -1
		}
		p.pendingEvents = append(p.pendingEvents, Event{
			Type:         EventEntitySample,
			Tick:         sample.Tick,
			GameTime:     sample.GameTime,
			Entity:       sample.Entity,
			PlayerSlot:   slot,
			EntitySample: &sample,
		})
	}
}

// appendAbilityChargeEvent emits a charge-count event when an ability
// entity's m_iRemainingCharges differs from the last value seen for it.
// Dash charges are the primary consumer; any charged ability flows here.
func (p *Parser) appendAbilityChargeEvent(tick uint32, e *Entity) {
	charges, ok := e.Int32("m_iRemainingCharges")
	if !ok {
		return
	}
	if last, seen := p.chargeLastSeen[e.index]; seen && last == charges {
		return
	}
	p.chargeLastSeen[e.index] = charges
	slot := int32(-1)
	if owner, ok := e.Int32("m_hOwnerEntity"); ok && owner >= 0 {
		ownerIndex := int32(uint32(owner) & uint32(entityHandleMask))
		if mapped, mappedOk := p.entityPlayerSlots[ownerIndex]; mappedOk {
			slot = mapped
		}
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventAbilityCharges,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     e.index,
		PlayerSlot: slot,
		AbilityCharges: &AbilityChargesEvent{
			Tick:             normalizedTick(tick),
			GameTime:         p.clock.GameTime(),
			ClassName:        e.class.name,
			RemainingCharges: charges,
		},
	})
}

func (p *Parser) updateEntityPlayerSlot(e *Entity) {
	for _, name := range []string{
		"m_iPlayerSlot",
		"m_nPlayerSlot",
		"m_iPlayerID",
		"m_nPlayerID",
		"m_iPlayerIndex",
		"m_nPlayerIndex",
	} {
		slot, ok := e.Int32(name)
		if ok && slot >= 0 {
			p.entityPlayerSlots[e.index] = slot
			return
		}
	}
	slot, ok := e.Int32("m_unLobbyPlayerSlot")
	if !ok || slot < 0 {
		return
	}
	for _, name := range []string{"m_hHeroPawn", "m_hPawn"} {
		handle, ok := e.Int32(name)
		if ok && handle >= 0 && uint32(handle) != invalidEntityHandle {
			p.entityPlayerSlots[int32(uint32(handle)&uint32(entityHandleMask))] = slot
			return
		}
	}
}

func isLikelyHeroClass(name string) bool {
	return stringsContains(name, "CitadelPlayerPawn") || stringsContains(name, "Hero")
}

func stringsContains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
