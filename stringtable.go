package s2replay

import (
	"strconv"

	"github.com/klauspost/compress/snappy"

	"github.com/paralin/s2replay/protocol"
)

const stringTableKeyHistorySize = 32

type stringTables struct {
	tables    map[int32]*stringTable
	nameIndex map[string]int32
	nextIndex int32
}

type stringTable struct {
	index             int32
	name              string
	items             map[int32]*stringTableItem
	userDataFixedSize bool
	userDataSizeBits  int32
	flags             int32
	varintBitCounts   bool
}

type stringTableItem struct {
	index int32
	key   string
	value []byte
}

func newStringTables() *stringTables {
	return &stringTables{
		tables:    make(map[int32]*stringTable),
		nameIndex: make(map[string]int32),
	}
}

func (ts *stringTables) getOrCreate(name string) *stringTable {
	if id, ok := ts.nameIndex[name]; ok {
		return ts.tables[id]
	}
	t := &stringTable{
		index: ts.nextIndex,
		name:  name,
		items: make(map[int32]*stringTableItem),
	}
	ts.nextIndex++
	ts.tables[t.index] = t
	ts.nameIndex[t.name] = t.index
	return t
}

func (p *Parser) applyCreateStringTable(msg *protocol.CSVCMsg_CreateStringTable) error {
	t := p.stringTables.getOrCreate(msg.GetName())
	t.userDataFixedSize = msg.GetUserDataFixedSize()
	t.userDataSizeBits = msg.GetUserDataSizeBits()
	t.flags = msg.GetFlags()
	t.varintBitCounts = msg.GetUsingVarintBitcounts()
	if t.items == nil {
		t.items = make(map[int32]*stringTableItem)
	}

	buf := msg.GetStringData()
	if msg.GetDataCompressed() {
		decoded, err := snappy.Decode(nil, buf)
		if err != nil {
			return nil
		}
		buf = decoded
	}
	items, err := parseStringTable(buf, msg.GetNumEntries(), t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)
	if err != nil {
		return nil
	}
	for _, item := range items {
		t.items[item.index] = item
	}
	if t.name == "instancebaseline" {
		p.updateInstanceBaseline()
	}
	return nil
}

func (p *Parser) applyUpdateStringTable(msg *protocol.CSVCMsg_UpdateStringTable) error {
	t := p.stringTables.tables[msg.GetTableId()]
	if t == nil {
		return errUnknownStringTable
	}
	items, err := parseStringTable(msg.GetStringData(), msg.GetNumChangedEntries(), t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)
	if err != nil {
		return nil
	}
	for _, item := range items {
		if old := t.items[item.index]; old != nil {
			if item.key != "" {
				old.key = item.key
			}
			if len(item.value) != 0 {
				old.value = item.value
			}
			continue
		}
		t.items[item.index] = item
	}
	if t.name == "instancebaseline" {
		p.updateInstanceBaseline()
	}
	return nil
}

func (p *Parser) applyDemoStringTables(msg *protocol.CDemoStringTables) {
	for _, incoming := range msg.GetTables() {
		t := p.stringTables.getOrCreate(incoming.GetTableName())
		if t == nil {
			continue
		}
		if incoming.TableFlags != nil {
			t.flags = incoming.GetTableFlags()
		}
		for i, item := range incoming.GetItems() {
			existing := t.items[int32(i)]
			if existing == nil {
				t.items[int32(i)] = &stringTableItem{
					index: int32(i),
					key:   item.GetStr(),
					value: item.GetData(),
				}
				continue
			}
			if item.Str != nil {
				existing.key = item.GetStr()
			}
			if item.Data != nil {
				existing.value = item.GetData()
			}
		}
		if t.name == "instancebaseline" {
			p.updateInstanceBaseline()
		}
	}
}

func (p *Parser) updateInstanceBaseline() {
	tableID, ok := p.stringTables.nameIndex["instancebaseline"]
	if !ok {
		return
	}
	table := p.stringTables.tables[tableID]
	if table == nil {
		return
	}
	for _, item := range table.items {
		classID, err := strconv.ParseInt(item.key, 10, 32)
		if err != nil {
			continue
		}
		p.classBaselines[int32(classID)] = item.value
	}
}

func parseStringTable(buf []byte, numUpdates int32, userDataFixed bool, userDataSizeBits int32, flags int32, varintBitCounts bool) ([]*stringTableItem, error) {
	r := newPacketReader(buf)
	items := make([]*stringTableItem, 0, numUpdates)
	keys := make([]string, 0, stringTableKeyHistorySize)
	index := int32(-1)
	for i := 0; i < int(numUpdates) && r.bitsRemaining() > 0; i++ {
		incr, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if incr {
			index++
		} else {
			v, err := r.readUvarint32()
			if err != nil {
				return nil, err
			}
			index = int32(v) + 1
		}
		key := ""
		hasKey, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if hasKey {
			useHistory, err := r.readBool()
			if err != nil {
				return nil, err
			}
			if useHistory {
				pos, err := r.readBits(5)
				if err != nil {
					return nil, err
				}
				size, err := r.readBits(5)
				if err != nil {
					return nil, err
				}
				if int(pos) < len(keys) {
					prev := keys[pos]
					if int(size) < len(prev) {
						key = prev[:size]
					} else {
						key = prev
					}
				}
			}
			suffix, err := r.readString()
			if err != nil {
				return nil, err
			}
			key += suffix
			if len(keys) >= stringTableKeyHistorySize {
				copy(keys, keys[1:])
				keys = keys[:len(keys)-1]
			}
			keys = append(keys, key)
		}
		value := []byte(nil)
		hasValue, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if hasValue {
			bits := int(userDataSizeBits)
			compressed := false
			if !userDataFixed {
				if flags&0x1 != 0 {
					var err error
					compressed, err = r.readBool()
					if err != nil {
						return nil, err
					}
				}
				if varintBitCounts {
					v, err := r.readUBitVar()
					if err != nil {
						return nil, err
					}
					bits = int(v) * 8
				} else {
					v, err := r.readBits(17)
					if err != nil {
						return nil, err
					}
					bits = int(v) * 8
				}
			}
			value, err = r.readBitsAsBytes(bits)
			if err != nil {
				return nil, err
			}
			if compressed {
				value, err = snappy.Decode(nil, value)
				if err != nil {
					return nil, err
				}
			}
		}
		items = append(items, &stringTableItem{index: index, key: key, value: value})
	}
	return items, nil
}
