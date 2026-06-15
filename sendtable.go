package s2replay

import "github.com/paralin/s2replay/protocol"

func (p *Parser) applySendTables(msg *protocol.CDemoSendTables) error {
	if len(msg.GetData()) == 0 {
		return nil
	}
	r := reader{buf: msg.GetData()}
	size, err := r.readUvarint()
	if err != nil {
		return p.applyFlattenedSerializers(msg.GetData())
	}
	buf, err := r.readBytes(int(size))
	if err != nil {
		return p.applyFlattenedSerializers(msg.GetData())
	}
	return p.applyFlattenedSerializers(buf)
}

func (p *Parser) applyFlattenedSerializers(buf []byte) error {
	flat := &protocol.CSVCMsg_FlattenedSerializer{}
	if err := flat.UnmarshalVT(buf); err != nil {
		return err
	}
	fields := make(map[int32]*field)
	fieldTypes := make(map[string]*fieldType)

	for _, raw := range flat.GetSerializers() {
		s := &serializer{
			name:    symbol(flat, raw.SerializerNameSym),
			version: raw.GetSerializerVersion(),
			fields:  make([]*field, 0, len(raw.GetFieldsIndex())),
		}
		for _, i := range raw.GetFieldsIndex() {
			f := fields[i]
			if f == nil {
				f = newField(flat, flat.GetFields()[i])
				if cached := fieldTypes[f.varType]; cached != nil {
					f.fieldType = cached
				} else {
					fieldTypes[f.varType] = f.fieldType
				}
				if f.serializerName != "" {
					f.serializer = p.serializers[f.serializerName]
				}
				if f.serializer != nil {
					if f.fieldType.pointer {
						f.setModel(fieldModelFixedTable)
					} else {
						f.setModel(fieldModelVariableTable)
					}
				} else if f.fieldType.baseType == "CUtlVector" || f.fieldType.baseType == "CNetworkUtlVectorBase" || f.fieldType.baseType == "CUtlVectorEmbeddedNetworkVar" {
					f.setModel(fieldModelVariableArray)
				} else if f.fieldType.count > 0 && f.fieldType.baseType != "char" {
					f.setModel(fieldModelFixedArray)
				} else {
					f.setModel(fieldModelSimple)
				}
				fields[i] = f
			}
			s.fields = append(s.fields, f)
		}
		p.serializers[s.name] = s
		if c := p.classesByName[s.name]; c != nil {
			c.serializer = s
		}
	}
	return nil
}

func symbol(msg *protocol.CSVCMsg_FlattenedSerializer, v *int32) string {
	if v == nil {
		return ""
	}
	i := int(*v)
	if i < 0 || i >= len(msg.GetSymbols()) {
		return ""
	}
	return msg.GetSymbols()[i]
}
