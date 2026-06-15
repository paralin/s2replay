package s2replay

import (
	"strconv"

	"github.com/paralin/s2replay/protocol"
)

const entityHandleMask uint64 = (1 << 14) - 1

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
	Tick        uint32  `json:"tick"`
	GameTime    float64 `json:"game_time"`
	Entity      int32   `json:"entity"`
	ClassID     int32   `json:"class_id"`
	ClassName   string  `json:"class_name"`
	Health      float32 `json:"health"`
	MaxHealth   float32 `json:"max_health"`
	Shield      float32 `json:"shield"`
	MaxShield   float32 `json:"max_shield"`
	PositionX   float32 `json:"position_x"`
	PositionY   float32 `json:"position_y"`
	PositionZ   float32 `json:"position_z"`
	HasHealth   bool    `json:"has_health"`
	HasShield   bool    `json:"has_shield"`
	HasPosition bool    `json:"has_position"`
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
	s.Health, s.HasHealth = firstFloat32(e,
		"m_iHealth",
		"m_iCurrentHealth",
		"m_flHealth",
		"m_CCitadelHealthComponent.m_iHealth",
	)
	s.MaxHealth, _ = firstFloat32(e,
		"m_iMaxHealth",
		"m_flMaxHealth",
		"m_CCitadelHealthComponent.m_iMaxHealth",
	)
	s.Shield, s.HasShield = firstFloat32(e,
		"m_iShield",
		"m_flShield",
		"m_CCitadelHealthComponent.m_iShield",
	)
	s.MaxShield, _ = firstFloat32(e,
		"m_iMaxShield",
		"m_flMaxShield",
		"m_CCitadelHealthComponent.m_iMaxShield",
	)
	x, okX := firstFloat32(e, "CBodyComponent.m_cellX", "m_CBodyComponent.m_cellX")
	y, okY := firstFloat32(e, "CBodyComponent.m_cellY", "m_CBodyComponent.m_cellY")
	z, okZ := firstFloat32(e, "CBodyComponent.m_cellZ", "m_CBodyComponent.m_cellZ")
	vx, vxOK := firstFloat32(e, "CBodyComponent.m_vecX", "m_CBodyComponent.m_vecX")
	vy, vyOK := firstFloat32(e, "CBodyComponent.m_vecY", "m_CBodyComponent.m_vecY")
	vz, vzOK := firstFloat32(e, "CBodyComponent.m_vecZ", "m_CBodyComponent.m_vecZ")
	if okX && okY && okZ && vxOK && vyOK && vzOK {
		s.PositionX = deadlockCoordFromCell(x, vx)
		s.PositionY = deadlockCoordFromCell(y, vy)
		s.PositionZ = deadlockCoordFromCell(z, vz)
		s.HasPosition = true
	}
	return s, s.HasHealth || s.HasShield || s.HasPosition
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

func (p *Parser) appendEntitySample(tick uint32, e *Entity) {
	if e == nil || e.class == nil || !e.active {
		return
	}
	if !isLikelyHeroClass(e.class.name) {
		return
	}
	if sample, ok := e.sample(tick, p.clock.GameTime()); ok {
		p.pendingSamples = append(p.pendingSamples, sample)
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
