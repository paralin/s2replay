package s2replay

type fieldState struct {
	values map[int]any
}

func newFieldState() *fieldState {
	return &fieldState{values: make(map[int]any)}
}

func (s *fieldState) get(fp fieldPath) any {
	cur := s
	for i := 0; i <= fp.last; i++ {
		v := cur.values[fp.path[i]]
		if i == fp.last {
			return v
		}
		next, ok := v.(*fieldState)
		if !ok {
			return nil
		}
		cur = next
	}
	return nil
}

func (s *fieldState) set(fp fieldPath, v any) {
	cur := s
	for i := 0; i <= fp.last; i++ {
		if i == fp.last {
			cur.values[fp.path[i]] = v
			return
		}
		next, ok := cur.values[fp.path[i]].(*fieldState)
		if !ok {
			next = newFieldState()
			cur.values[fp.path[i]] = next
		}
		cur = next
	}
}
