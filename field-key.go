package s2replay

// fieldKey names a field the parser reads on every sample. Each key is
// registered once at package init and carries a dense id, so a class
// resolves it to a field path through one slice index instead of hashing
// the dotted name on every read.
type fieldKey struct {
	id   int
	name string
}

// fieldKeyCount is the number of registered keys. Registration happens only
// during package init, so the count is fixed before any parser runs.
var fieldKeyCount int

// newFieldKey registers name. Call it only from package-level var
// initializers.
func newFieldKey(name string) fieldKey {
	k := fieldKey{id: fieldKeyCount, name: name}
	fieldKeyCount++
	return k
}

// fieldKeys registers each name in order, for reads that try alternatives.
func fieldKeys(names ...string) []fieldKey {
	keys := make([]fieldKey, len(names))
	for i, name := range names {
		keys[i] = newFieldKey(name)
	}
	return keys
}

// keyedPath is a class's resolution of one fieldKey.
type keyedPath struct {
	namedPath
	resolved bool
}

// pathForKey resolves k to its field path in this class, caching the result
// by key id. The returned pointer stays valid for the class's lifetime.
func (c *entityClass) pathForKey(k fieldKey) *namedPath {
	if c.keyed == nil {
		c.keyed = make([]keyedPath, fieldKeyCount)
	}
	kp := &c.keyed[k.id]
	if !kp.resolved {
		kp.path, kp.found = c.pathForName(k.name)
		kp.resolved = true
	}
	return &kp.namedPath
}
