package s2replay

import (
	"github.com/paralin/s2replay/protocol"
)

// entityClass describes one networked entity class.
type entityClass struct {
	// id is the networked class id.
	id int32
	// name is the networked class name.
	name string
	// serializer defines the class's fields; nil before SendTables arrive.
	serializer *serializer
}

// pathForName resolves a dotted field name to its field path, reporting
// whether the class's serializer contains it.
func (c *entityClass) pathForName(name string) (fieldPath, bool) {
	var fp fieldPath
	fp.path = [fieldPathMaxDepth]int{-1}
	if c.serializer == nil {
		return fp, false
	}
	if c.serializer.findPath(splitFieldName(name), &fp, 0) {
		return fp, true
	}
	return fp, false
}

// fieldName joins a field path back into its dotted field name.
func (c *entityClass) fieldName(fp fieldPath) string {
	if c.serializer == nil {
		return ""
	}
	return joinFieldName(c.serializer.nameByPath(fp, 0))
}

// fieldByPath returns the field descriptor at the given path, or nil when the
// path is invalid for this class.
func (c *entityClass) fieldByPath(fp fieldPath) *field {
	if c.serializer == nil {
		return nil
	}
	return c.serializer.fieldByPath(fp, 0)
}

// rootField returns the top-level field containing the given path, or nil
// when the path has no root.
func (c *entityClass) rootField(fp fieldPath) *field {
	if c.serializer == nil || fp.path[0] < 0 || fp.path[0] >= len(c.serializer.fields) {
		return nil
	}
	return c.serializer.fields[fp.path[0]]
}

// decoder returns the field decoder for the given path, or nil when the class
// has no serializer or the path is invalid.
func (c *entityClass) decoder(fp fieldPath) fieldDecoder {
	if c.serializer == nil {
		return nil
	}
	return c.serializer.decoderByPath(fp, 0)
}

// applyServerInfo records the tick interval, game directory, and map name,
// and derives the class id bit width from the class limit.
func (p *Parser) applyServerInfo(msg *protocol.CSVCMsg_ServerInfo) {
	p.clock.SetInterval(float64(msg.GetTickInterval()))
	p.serverGame = msg.GetGameDir()
	p.serverMap = msg.GetMapName()
	if maxClasses := msg.GetMaxClasses(); maxClasses > 0 {
		p.classIDBits = bitsForClassLimit(maxClasses)
	}
}

// applyDemoClassInfo records the classes advertised by a DEM_ClassInfo
// message and refreshes the instance baseline.
func (p *Parser) applyDemoClassInfo(msg *protocol.CDemoClassInfo) {
	if p.classIDBits == 0 {
		p.classIDBits = bitsForDemoClasses(msg.GetClasses())
	}
	for _, raw := range msg.GetClasses() {
		c := &entityClass{
			id:         raw.GetClassId(),
			name:       raw.GetNetworkName(),
			serializer: p.serializers[raw.GetNetworkName()],
		}
		p.classesByID[c.id] = c
		p.classesByName[c.name] = c
	}
	p.updateInstanceBaseline()
}

// applySvcClassInfo records the classes advertised by a SVC_ClassInfo message
// and refreshes the instance baseline.
func (p *Parser) applySvcClassInfo(msg *protocol.CSVCMsg_ClassInfo) {
	if p.classIDBits == 0 {
		p.classIDBits = bitsForSvcClasses(msg.GetClasses())
	}
	for _, raw := range msg.GetClasses() {
		c := &entityClass{
			id:         raw.GetClassId(),
			name:       raw.GetClassName(),
			serializer: p.serializers[raw.GetClassName()],
		}
		p.classesByID[c.id] = c
		p.classesByName[c.name] = c
	}
	p.updateInstanceBaseline()
}

// bitsForDemoClasses returns the bit width needed to encode the class ids in
// a DEM_ClassInfo message.
func bitsForDemoClasses(classes []*protocol.CDemoClassInfoClassT) uint8 {
	var maxID int32
	for _, c := range classes {
		if id := c.GetClassId(); id > maxID {
			maxID = id
		}
	}
	return bitsForClassLimit(maxID)
}

// bitsForSvcClasses returns the bit width needed to encode the class ids in a
// SVC_ClassInfo message.
func bitsForSvcClasses(classes []*protocol.CSVCMsg_ClassInfoClassT) uint8 {
	var maxID int32
	for _, c := range classes {
		if id := c.GetClassId(); id > maxID {
			maxID = id
		}
	}
	return bitsForClassLimit(maxID)
}

// bitsForClassLimit returns the number of bits needed to represent n, with a
// minimum of one.
func bitsForClassLimit(n int32) uint8 {
	var bits uint8
	v := n
	for v > 0 {
		bits++
		v >>= 1
	}
	if bits == 0 {
		return 1
	}
	return bits
}

// splitFieldName splits a dotted field name into its components.
func splitFieldName(name string) []string {
	if name == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			parts = append(parts, name[start:i])
			start = i + 1
		}
	}
	return append(parts, name[start:])
}

// joinFieldName joins field path components back into a dotted field name.
func joinFieldName(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts) - 1
	for _, part := range parts {
		n += len(part)
	}
	b := make([]byte, 0, n)
	for i, part := range parts {
		if i != 0 {
			b = append(b, '.')
		}
		b = append(b, part...)
	}
	return string(b)
}
