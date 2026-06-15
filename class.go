package s2replay

import (
	"github.com/paralin/s2replay/protocol"
)

type entityClass struct {
	id         int32
	name       string
	serializer *serializer
}

func (c *entityClass) pathForName(name string) (fieldPath, bool) {
	var fp fieldPath
	fp.path = [7]int{-1, 0, 0, 0, 0, 0, 0}
	if c.serializer == nil {
		return fp, false
	}
	if c.serializer.findPath(splitFieldName(name), &fp, 0) {
		return fp, true
	}
	return fp, false
}

func (c *entityClass) fieldName(fp fieldPath) string {
	if c.serializer == nil {
		return ""
	}
	return joinFieldName(c.serializer.nameByPath(fp, 0))
}

func (c *entityClass) fieldByPath(fp fieldPath) *field {
	if c.serializer == nil {
		return nil
	}
	return c.serializer.fieldByPath(fp, 0)
}

func (c *entityClass) rootField(fp fieldPath) *field {
	if c.serializer == nil || fp.path[0] < 0 || fp.path[0] >= len(c.serializer.fields) {
		return nil
	}
	return c.serializer.fields[fp.path[0]]
}

func (c *entityClass) decoder(fp fieldPath) fieldDecoder {
	if c.serializer == nil {
		return nil
	}
	return c.serializer.decoderByPath(fp, 0)
}

func (p *Parser) applyServerInfo(msg *protocol.CSVCMsg_ServerInfo) {
	p.clock.SetInterval(float64(msg.GetTickInterval()))
	if maxClasses := msg.GetMaxClasses(); maxClasses > 0 {
		p.classIDBits = bitsForClassLimit(maxClasses)
	}
}

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

func bitsForDemoClasses(classes []*protocol.CDemoClassInfoClassT) uint8 {
	var maxID int32
	for _, c := range classes {
		if id := c.GetClassId(); id > maxID {
			maxID = id
		}
	}
	return bitsForClassLimit(maxID)
}

func bitsForSvcClasses(classes []*protocol.CSVCMsg_ClassInfoClassT) uint8 {
	var maxID int32
	for _, c := range classes {
		if id := c.GetClassId(); id > maxID {
			maxID = id
		}
	}
	return bitsForClassLimit(maxID)
}

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
