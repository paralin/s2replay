package s2replay

import (
	"math"
	"strconv"
	"strings"

	"github.com/paralin/s2replay/protocol"
)

const (
	fieldModelSimple = iota
	fieldModelFixedArray
	fieldModelFixedTable
	fieldModelVariableArray
	fieldModelVariableTable
)

type fieldDecoder func(*packetReader) (any, error)

type field struct {
	varName        string
	varType        string
	sendNode       string
	serializerName string
	encoder        string
	encodeFlags    *int32
	bitCount       *int32
	lowValue       *float32
	highValue      *float32
	fieldType      *fieldType
	serializer     *serializer
	model          int
	decoder        fieldDecoder
	baseDecoder    fieldDecoder
	childDecoder   fieldDecoder
}

type serializer struct {
	name    string
	version int32
	fields  []*field
}

func newField(msg *protocol.CSVCMsg_FlattenedSerializer, f *protocol.ProtoFlattenedSerializerFieldT) *field {
	resolve := func(v *int32) string {
		if v == nil {
			return ""
		}
		i := int(*v)
		if i < 0 || i >= len(msg.GetSymbols()) {
			return ""
		}
		return msg.GetSymbols()[i]
	}
	x := &field{
		varName:        resolve(f.VarNameSym),
		varType:        resolve(f.VarTypeSym),
		sendNode:       resolve(f.SendNodeSym),
		serializerName: resolve(f.FieldSerializerNameSym),
		encoder:        resolve(f.VarEncoderSym),
		encodeFlags:    f.EncodeFlags,
		bitCount:       f.BitCount,
		lowValue:       f.LowValue,
		highValue:      f.HighValue,
	}
	if x.sendNode == "(root)" {
		x.sendNode = ""
	}
	switch x.varName {
	case "m_flSimulationTime", "m_flAnimTime":
		x.encoder = "simtime"
	case "m_flRuneTime":
		x.encoder = "runetime"
	}
	x.fieldType = newFieldType(x.varType)
	return x
}

func (f *field) setModel(model int) {
	f.model = model
	switch model {
	case fieldModelFixedArray:
		f.decoder = findFieldDecoder(f)
	case fieldModelFixedTable:
		f.baseDecoder = boolDecoder
	case fieldModelVariableArray:
		f.baseDecoder = uintDecoder
		if f.fieldType.genericType != nil {
			child := *f
			child.varType = f.fieldType.genericType.baseType
			child.fieldType = f.fieldType.genericType
			f.childDecoder = findFieldDecoder(&child)
		} else {
			f.childDecoder = uintDecoder
		}
	case fieldModelVariableTable:
		f.baseDecoder = uintDecoder
	case fieldModelSimple:
		f.decoder = findFieldDecoder(f)
	}
}

func (s *serializer) fieldByPath(fp fieldPath, pos int) *field {
	if pos > fp.last || fp.path[pos] < 0 || fp.path[pos] >= len(s.fields) {
		return nil
	}
	return s.fields[fp.path[pos]].fieldByPath(fp, pos+1)
}

func (f *field) fieldByPath(fp fieldPath, pos int) *field {
	switch f.model {
	case fieldModelFixedArray:
		return f
	case fieldModelFixedTable:
		if fp.last != pos-1 && f.serializer != nil {
			return f.serializer.fieldByPath(fp, pos)
		}
	case fieldModelVariableArray:
		return f
	case fieldModelVariableTable:
		if fp.last >= pos+1 && f.serializer != nil {
			return f.serializer.fieldByPath(fp, pos+1)
		}
	}
	return f
}

func (s *serializer) nameByPath(fp fieldPath, pos int) []string {
	if pos > fp.last || fp.path[pos] < 0 || fp.path[pos] >= len(s.fields) {
		return nil
	}
	return s.fields[fp.path[pos]].nameByPath(fp, pos+1)
}

func (f *field) nameByPath(fp fieldPath, pos int) []string {
	parts := make([]string, 0, 4)
	if f.sendNode != "" {
		parts = append(parts, strings.Split(f.sendNode, ".")...)
	}
	parts = append(parts, f.varName)
	switch f.model {
	case fieldModelFixedArray:
		if fp.last == pos {
			parts = append(parts, padFieldIndex(fp.path[pos]))
		}
	case fieldModelFixedTable:
		if fp.last >= pos && f.serializer != nil {
			parts = append(parts, f.serializer.nameByPath(fp, pos)...)
		}
	case fieldModelVariableArray:
		if fp.last == pos {
			parts = append(parts, padFieldIndex(fp.path[pos]))
		}
	case fieldModelVariableTable:
		if fp.last != pos-1 {
			parts = append(parts, padFieldIndex(fp.path[pos]))
			if fp.last != pos && f.serializer != nil {
				parts = append(parts, f.serializer.nameByPath(fp, pos+1)...)
			}
		}
	}
	return parts
}

func (s *serializer) decoderByPath(fp fieldPath, pos int) fieldDecoder {
	if pos > fp.last || fp.path[pos] < 0 || fp.path[pos] >= len(s.fields) {
		return nil
	}
	return s.fields[fp.path[pos]].decoderByPath(fp, pos+1)
}

func (f *field) decoderByPath(fp fieldPath, pos int) fieldDecoder {
	switch f.model {
	case fieldModelFixedArray:
		return f.decoder
	case fieldModelFixedTable:
		if fp.last == pos-1 {
			return f.baseDecoder
		}
		if f.serializer != nil {
			if d := f.serializer.decoderByPath(fp, pos); d != nil {
				return d
			}
		}
		return f.baseDecoder
	case fieldModelVariableArray:
		if fp.last == pos {
			return f.childDecoder
		}
		return f.baseDecoder
	case fieldModelVariableTable:
		if fp.last >= pos+1 && f.serializer != nil {
			return f.serializer.decoderByPath(fp, pos+1)
		}
		return f.baseDecoder
	}
	return f.decoder
}

func (s *serializer) findPath(parts []string, fp *fieldPath, pos int) bool {
	if len(parts) == 0 {
		return false
	}
	for i, f := range s.fields {
		if f.matchPath(parts, fp, pos, i) {
			return true
		}
	}
	return false
}

func (f *field) matchPath(parts []string, fp *fieldPath, pos, index int) bool {
	nameParts := []string{f.varName}
	if f.sendNode != "" {
		nameParts = append(strings.Split(f.sendNode, "."), f.varName)
	}
	if len(parts) < len(nameParts) || !sameStrings(parts[:len(nameParts)], nameParts) {
		return false
	}
	fp.path[pos] = index
	rest := parts[len(nameParts):]
	switch f.model {
	case fieldModelFixedTable:
		if len(rest) == 0 {
			fp.last = pos
			return true
		}
		if f.serializer == nil {
			return false
		}
		return f.serializer.findPath(rest, fp, pos)
	case fieldModelVariableTable:
		if len(rest) < 2 || f.serializer == nil {
			return false
		}
		n, err := strconv.Atoi(rest[0])
		if err != nil {
			return false
		}
		fp.path[pos] = n
		fp.last = pos + 1
		return f.serializer.findPath(rest[1:], fp, pos+1)
	case fieldModelFixedArray, fieldModelVariableArray:
		if len(rest) != 1 {
			return false
		}
		n, err := strconv.Atoi(rest[0])
		if err != nil {
			return false
		}
		fp.path[pos] = n
		fp.last = pos
		return true
	case fieldModelSimple:
		fp.last = pos
		return len(rest) == 0
	default:
		return false
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func padFieldIndex(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func findFieldDecoder(f *field) fieldDecoder {
	switch f.fieldType.baseType {
	case "float32":
		return floatDecoder(f)
	case "CNetworkedQuantizedFloat":
		return quantizedDecoder(f)
	case "Vector", "VectorWS":
		return vectorDecoder(3, f)
	case "Vector2D":
		return vectorDecoder(2, f)
	case "Vector4D":
		return vectorDecoder(4, f)
	case "uint64", "CStrongHandle", "HeroFacetKey_t", "ResourceId_t":
		if f.encoder == "fixed64" {
			return fixed64Decoder
		}
		return uint64Decoder
	case "bool":
		return boolDecoder
	case "char", "CUtlString", "CUtlSymbolLarge":
		return stringDecoder
	case "int8", "int16", "int32", "int64":
		return intDecoder
	case "uint8", "uint16", "uint32", "color32", "BloodType", "CUtlStringToken", "CHandle", "CEntityHandle", "CGameSceneNodeHandle":
		return uintDecoder
	case "GameTime_t":
		return noScaleDecoder
	case "QAngle":
		return qangleDecoder(f)
	case "CBodyComponent", "CPhysicsComponent", "CRenderComponent":
		return componentDecoder
	default:
		return uintDecoder
	}
}

func findDecoderByBase(base string) fieldDecoder {
	switch base {
	case "float32", "CNetworkedQuantizedFloat":
		return noScaleDecoder
	case "bool":
		return boolDecoder
	case "int8", "int16", "int32", "int64":
		return intDecoder
	case "char", "CUtlString", "CUtlSymbolLarge":
		return stringDecoder
	default:
		return uintDecoder
	}
}

func floatDecoder(f *field) fieldDecoder {
	switch f.encoder {
	case "coord":
		return coordDecoder
	case "simtime":
		return simulationTimeDecoder
	case "runetime":
		return runeTimeDecoder
	}
	if f.bitCount == nil || *f.bitCount <= 0 || *f.bitCount >= 32 {
		return noScaleDecoder
	}
	return quantizedDecoder(f)
}

func quantizedDecoder(f *field) fieldDecoder {
	q := newQuantizedFloatDecoder(f.bitCount, f.encodeFlags, f.lowValue, f.highValue)
	return func(r *packetReader) (any, error) { return q.decode(r) }
}

func vectorDecoder(n int, f *field) fieldDecoder {
	if n == 3 && f.encoder == "normal" {
		return func(r *packetReader) (any, error) { return r.read3BitNormal() }
	}
	d := floatDecoder(f)
	return func(r *packetReader) (any, error) {
		v := make([]float32, n)
		for i := range v {
			x, err := d(r)
			if err != nil {
				return nil, err
			}
			v[i] = x.(float32)
		}
		return v, nil
	}
}

func qangleDecoder(f *field) fieldDecoder {
	if f.encoder == "qangle_pitch_yaw" && f.bitCount != nil {
		n := uint8(*f.bitCount)
		return func(r *packetReader) (any, error) {
			x, err := r.readAngle(n)
			if err != nil {
				return nil, err
			}
			y, err := r.readAngle(n)
			return []float32{x, y, 0}, err
		}
	}
	if f.encoder == "qangle_precise" {
		return func(r *packetReader) (any, error) {
			v := make([]float32, 3)
			hasX, err := r.readBool()
			if err != nil {
				return nil, err
			}
			hasY, err := r.readBool()
			if err != nil {
				return nil, err
			}
			hasZ, err := r.readBool()
			if err != nil {
				return nil, err
			}
			if hasX {
				v[0], err = r.readAngle(20)
				if err != nil {
					return nil, err
				}
			}
			if hasY {
				v[1], err = r.readAngle(20)
				if err != nil {
					return nil, err
				}
			}
			if hasZ {
				v[2], err = r.readAngle(20)
			}
			return v, err
		}
	}
	if f.bitCount != nil && *f.bitCount != 0 && *f.bitCount != 32 {
		n := uint8(*f.bitCount)
		return func(r *packetReader) (any, error) {
			x, err := r.readAngle(n)
			if err != nil {
				return nil, err
			}
			y, err := r.readAngle(n)
			if err != nil {
				return nil, err
			}
			z, err := r.readAngle(n)
			return []float32{x, y, z}, err
		}
	}
	if f.bitCount != nil && *f.bitCount == 32 {
		return func(r *packetReader) (any, error) {
			v := make([]float32, 3)
			var err error
			for i := range v {
				v[i], err = r.readFloat32()
				if err != nil {
					return nil, err
				}
			}
			return v, nil
		}
	}
	return func(r *packetReader) (any, error) {
		v := make([]float32, 3)
		hasX, err := r.readBool()
		if err != nil {
			return nil, err
		}
		hasY, err := r.readBool()
		if err != nil {
			return nil, err
		}
		hasZ, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if hasX {
			v[0], err = r.readCoord()
			if err != nil {
				return nil, err
			}
		}
		if hasY {
			v[1], err = r.readCoord()
			if err != nil {
				return nil, err
			}
		}
		if hasZ {
			v[2], err = r.readCoord()
		}
		return v, err
	}
}

func boolDecoder(r *packetReader) (any, error) {
	return r.readBool()
}

func stringDecoder(r *packetReader) (any, error) {
	return r.readString()
}

func uintDecoder(r *packetReader) (any, error) {
	return r.readUvarint32()
}

func uint64Decoder(r *packetReader) (any, error) {
	return r.readUvarint64()
}

func fixed64Decoder(r *packetReader) (any, error) {
	return r.readLEUint64()
}

func intDecoder(r *packetReader) (any, error) {
	return r.readVarint32()
}

func noScaleDecoder(r *packetReader) (any, error) {
	return r.readFloat32()
}

func coordDecoder(r *packetReader) (any, error) {
	return r.readCoord()
}

func simulationTimeDecoder(r *packetReader) (any, error) {
	v, err := r.readUvarint32()
	return float32(v) * (1.0 / 30), err
}

func runeTimeDecoder(r *packetReader) (any, error) {
	v, err := r.readBits(4)
	return math.Float32frombits(v), err
}

func componentDecoder(r *packetReader) (any, error) {
	return r.readBits(1)
}
