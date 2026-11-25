package s2replay

// fieldState holds one serializer level of an entity's decoded fields. Slots
// are indexed directly by field path component, which Source 2 numbers
// densely from zero, so a read or write is one slice index per path level.
// A slot whose field nests a serializer or a vector holds the child level.
type fieldState struct {
	slots []fieldSlot
}

// fieldSlot is one field: its value, or the nested level beneath it, and the
// tick at which it was last written.
type fieldSlot struct {
	value any
	child *fieldState
	tick  uint32
	set   bool
}

func newFieldState() *fieldState {
	return &fieldState{}
}

// get returns the value at fp and the tick it was last written, or false when
// nothing has been written there.
func (s *fieldState) get(fp *fieldPath) (any, uint32, bool) {
	cur := s
	for i := 0; ; i++ {
		idx := fp.path[i]
		if idx < 0 || idx >= len(cur.slots) {
			return nil, 0, false
		}
		slot := &cur.slots[idx]
		if i == fp.last {
			return slot.value, slot.tick, slot.set
		}
		if slot.child == nil {
			return nil, 0, false
		}
		cur = slot.child
	}
}

// set stores v at fp with the tick it was written, creating levels as needed.
func (s *fieldState) set(fp fieldPath, v any, tick uint32) {
	cur := s
	for i := 0; ; i++ {
		slot := cur.slot(fp.path[i])
		if i == fp.last {
			slot.value, slot.tick, slot.set = v, tick, true
			if slot.child != nil {
				slot.child.resize(v)
			}
			return
		}
		if slot.child == nil {
			slot.child = newFieldState()
		}
		cur = slot.child
	}
}

// resize applies a write to the field that owns this level: a vector's new
// length drops the elements past it, and a cleared pointer drops its fields.
// Elements below the length keep their values, since the server resends only
// the elements that changed.
func (s *fieldState) resize(v any) {
	n := len(s.slots)
	switch v := v.(type) {
	case uint32:
		n = min(n, int(v))
	case uint64:
		n = int(min(uint64(n), v))
	case bool:
		if !v {
			n = 0
		}
	}
	clear(s.slots[n:])
	s.slots = s.slots[:n]
}

// slot returns the slot at idx, growing the level to hold it.
func (s *fieldState) slot(idx int) *fieldSlot {
	if idx >= len(s.slots) {
		if idx < cap(s.slots) {
			s.slots = s.slots[:idx+1]
		} else {
			grown := make([]fieldSlot, idx+1, max(idx+1, 2*cap(s.slots), 4))
			copy(grown, s.slots)
			s.slots = grown
		}
	}
	return &s.slots[idx]
}

// clone returns a deep copy of s whose levels and slots share two
// allocations. Values are shared: a decoder returns a fresh value for every
// write and nothing mutates one in place.
func (s *fieldState) clone() *fieldState {
	slots, levels := s.size()
	slotArena := make([]fieldSlot, slots)
	levelArena := make([]fieldState, levels)
	return s.cloneInto(&slotArena, &levelArena)
}

// size counts the slots and levels beneath and including s.
func (s *fieldState) size() (slots, levels int) {
	slots, levels = len(s.slots), 1
	for i := range s.slots {
		if c := s.slots[i].child; c != nil {
			cs, cl := c.size()
			slots, levels = slots+cs, levels+cl
		}
	}
	return slots, levels
}

// cloneInto copies s into the front of the arenas and advances them. Each
// level's slots are capped at their length, so a level that later grows
// reallocates instead of writing into its neighbor.
func (s *fieldState) cloneInto(slots *[]fieldSlot, levels *[]fieldState) *fieldState {
	dst := &(*levels)[0]
	*levels = (*levels)[1:]
	n := len(s.slots)
	dst.slots = (*slots)[:n:n]
	*slots = (*slots)[n:]
	copy(dst.slots, s.slots)
	for i := range dst.slots {
		if c := dst.slots[i].child; c != nil {
			dst.slots[i].child = c.cloneInto(slots, levels)
		}
	}
	return dst
}
