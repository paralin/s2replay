package s2replay

import (
	"strconv"

	"github.com/klauspost/compress/snappy"
	"github.com/paralin/s2replay/protocol"
)

const (
	// stringTableKeyHistorySize is the rolling prefix dictionary size.
	stringTableKeyHistorySize = 32
	// maxStringTableUpdates bounds allocations for a declared update batch.
	maxStringTableUpdates = 1 << 20
	// maxStringTableIndex is the largest accepted 20-bit entry index.
	maxStringTableIndex = maxStringTableUpdates - 1
	// maxStringTableKeyBytes limits a reconstructed network-string key.
	maxStringTableKeyBytes = 1 << 10
	// maxStringTableUserDataBytes bounds each raw or decompressed entry value.
	maxStringTableUserDataBytes = 1 << 14
	// maxStringTableDataBytes bounds expansion of a compressed table payload.
	maxStringTableDataBytes = 1 << 24
)

// stringTables holds every string table seen in the demo, addressable by
// creation order and by name.
type stringTables struct {
	// tables maps table id to the table.
	tables map[int32]*stringTable
	// nameIndex maps table name to table id.
	nameIndex map[string]int32
	// nextIndex is the id assigned to the next created table.
	nextIndex int32
}

// stringTable is one network string table: its wire decode settings and the
// items observed so far.
type stringTable struct {
	// index is the table id assigned in creation order.
	index int32
	// name is the table's network name, such as instancebaseline.
	name string
	// items maps entry index to the current item state.
	items map[int32]*stringTableItem
	// userDataFixedSize indicates whether user data is a fixed bit count.
	userDataFixedSize bool
	// userDataSizeBits is the user-data bit count for fixed-size entries.
	userDataSizeBits int32
	// flags holds the table's wire flags, including the compressed-value bit.
	flags int32
	// varintBitCounts indicates value byte counts use ubitvar encoding.
	varintBitCounts bool
}

// stringTableItem is one string table entry: its index, key, and user data.
type stringTableItem struct {
	// index identifies the entry within its table.
	index int32
	// key is the reconstructed network-string key.
	key string
	// value is the decoded user data for this entry.
	value []byte
}

// newStringTables constructs an empty string table set.
func newStringTables() *stringTables {
	return &stringTables{
		tables:    make(map[int32]*stringTable),
		nameIndex: make(map[string]int32),
	}
}

// getOrCreate returns the named table, creating it with the next free id
// when it does not yet exist.
func (ts *stringTables) getOrCreate(name string) *stringTable {
	if id, ok := ts.nameIndex[name]; ok {
		return ts.tables[id]
	}

	// Create the table and register it under both id and name.
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

// applyCreateStringTable applies a svc_CreateStringTable message, decoding
// the table's initial contents and activating any derived state it drives.
func (p *Parser) applyCreateStringTable(tick uint32, msg *protocol.CSVCMsg_CreateStringTable) error {
	// Record the table's decode settings, reusing an existing table entry
	// created earlier by a full-update snapshot.
	t := p.stringTables.getOrCreate(msg.GetName())
	t.userDataFixedSize = msg.GetUserDataFixedSize()
	t.userDataSizeBits = msg.GetUserDataSizeBits()
	t.flags = msg.GetFlags()
	t.varintBitCounts = msg.GetUsingVarintBitcounts()
	if t.items == nil {
		t.items = make(map[int32]*stringTableItem)
	}

	// Decompress the table payload before decoding when it is snappy-compressed.
	buf := msg.GetStringData()
	if msg.GetDataCompressed() {
		decodedLen, err := snappy.DecodedLen(buf)
		if err != nil {
			return err
		}
		if decodedLen > maxStringTableDataBytes {
			return errStringTableDataTooLarge
		}
		buf, err = snappy.Decode(nil, buf)
		if err != nil {
			return err
		}
	}

	// Decode the initial items and register them in the table.
	items, err := parseStringTable(buf, msg.GetNumEntries(), t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)
	if err != nil {
		return err
	}
	for _, item := range items {
		t.items[item.index] = item
		if t.name == "ActiveModifiers" {
			if err := p.applyActiveModifierItem(tick, item); err != nil {
				return err
			}
		}
	}

	// instancebaseline changes drive every entity class's default state.
	if t.name == "instancebaseline" {
		p.updateInstanceBaseline()
	}
	return nil
}

// applyUpdateStringTable applies a svc_UpdateStringTable message, patching
// the changed entries into the table identified by the message.
func (p *Parser) applyUpdateStringTable(tick uint32, msg *protocol.CSVCMsg_UpdateStringTable) error {
	t := p.stringTables.tables[msg.GetTableId()]
	if t == nil {
		return errUnknownStringTable
	}

	// Decode the changed entries and patch them into the table, preserving
	// fields an incremental update does not carry.
	items, err := parseStringTable(msg.GetStringData(), msg.GetNumChangedEntries(), t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)
	if err != nil {
		return err
	}
	for _, item := range items {
		if old := t.items[item.index]; old != nil {
			if item.key != "" {
				old.key = item.key
			}
			if len(item.value) != 0 {
				old.value = item.value
			}

			// Updated modifiers regenerate their event from the merged state.
			if t.name == "ActiveModifiers" {
				if err := p.applyActiveModifierItem(tick, old); err != nil {
					return err
				}
			}
			continue
		}
		t.items[item.index] = item
		if t.name == "ActiveModifiers" {
			if err := p.applyActiveModifierItem(tick, item); err != nil {
				return err
			}
		}
	}

	// instancebaseline changes drive every entity class's default state.
	if t.name == "instancebaseline" {
		p.updateInstanceBaseline()
	}
	return nil
}

// applyDemoStringTables applies a CDemoStringTables full-update snapshot,
// creating tables and items the snapshot names and merging it into state.
func (p *Parser) applyDemoStringTables(tick uint32, msg *protocol.CDemoStringTables) error {
	for _, incoming := range msg.GetTables() {
		t := p.stringTables.getOrCreate(incoming.GetTableName())
		if t == nil {
			continue
		}

		// Merge the snapshot into the table: present fields overwrite, absent
		// fields keep their current state.
		if incoming.TableFlags != nil {
			t.flags = incoming.GetTableFlags()
		}
		for i, item := range incoming.GetItems() {
			existing := t.items[int32(i)]
			if existing == nil {
				existing = &stringTableItem{
					index: int32(i),
					key:   item.GetStr(),
					value: item.GetData(),
				}
				t.items[int32(i)] = existing
				if t.name == "ActiveModifiers" {
					if err := p.applyActiveModifierItem(tick, existing); err != nil {
						return err
					}
				}
				continue
			}
			if item.Str != nil {
				existing.key = item.GetStr()
			}
			if item.Data != nil {
				existing.value = item.GetData()
			}
			if t.name == "ActiveModifiers" {
				if err := p.applyActiveModifierItem(tick, existing); err != nil {
					return err
				}
			}
		}

		// instancebaseline changes drive every entity class's default state.
		if t.name == "instancebaseline" {
			p.updateInstanceBaseline()
		}
	}
	return nil
}

// updateInstanceBaseline rebuilds the parser's class baseline table from the
// current contents of the instancebaseline string table.
func (p *Parser) updateInstanceBaseline() {
	tableID, ok := p.stringTables.nameIndex["instancebaseline"]
	if !ok {
		return
	}

	// Take the current baseline data from each entry, keyed by class id.
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

// parseStringTable decodes a string-table payload carrying numUpdates entries
// with the table's wire settings, enforcing Source size limits on indexes,
// keys, and user data.
func parseStringTable(buf []byte, numUpdates int32, userDataFixed bool, userDataSizeBits int32, flags int32, varintBitCounts bool) ([]*stringTableItem, error) {
	// Validate declared sizes before allocating result storage or converting lengths.
	if numUpdates < 0 {
		return nil, errInvalidStringTableUpdateCount
	}
	if numUpdates > maxStringTableUpdates || int64(numUpdates) > int64(len(buf))*8 {
		return nil, errStringTableUpdateCountTooLarge
	}
	if userDataSizeBits < 0 {
		return nil, errInvalidStringTableUserDataSize
	}
	if userDataFixed && userDataSizeBits > maxStringTableUserDataBytes*8 {
		return nil, errStringTableUserDataTooLarge
	}

	// Decode exactly the declared number of entries; an exhausted payload is an error.
	r := newPacketReader(buf)
	items := make([]*stringTableItem, 0, int(numUpdates))
	keys := make([]string, 0, stringTableKeyHistorySize)
	index := int32(-1)
	for range numUpdates {
		// Decode the entry index: an implicit increment or an explicit value.
		incr, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if incr {
			if index >= maxStringTableIndex {
				return nil, errStringTableIndexTooLarge
			}
			index++
		} else {
			v, err := r.readUvarint32()
			if err != nil {
				return nil, err
			}
			// Explicit entries encode a gap from the preceding index; the
			// accumulated result, not the wire value, is what needs bounding.
			index += int32(v) + 2
			if index >= maxStringTableIndex {
				return nil, errStringTableIndexTooLarge
			}
		}

		// Decode the entry key, either a history reference plus suffix or a
		// full inline string.
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

			// Decode the key's suffix and record the complete key in history.
			suffix, err := r.readStringMax(maxStringTableKeyBytes - len(key))
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

		// Decode the entry's user data, which may be omitted entirely.
		value := []byte(nil)
		hasValue, err := r.readBool()
		if err != nil {
			return nil, err
		}
		if hasValue {
			bits := uint64(userDataSizeBits)
			compressed := false
			if !userDataFixed {
				if flags&0x1 != 0 {
					compressed, err = r.readBool()
					if err != nil {
						return nil, err
					}
				}

				// Variable-length user data carries its byte count on the wire.
				var bytes uint32
				if varintBitCounts {
					bytes, err = r.readUBitVar()
				} else {
					bytes, err = r.readBits(17)
				}
				if err != nil {
					return nil, err
				}
				bits = uint64(bytes) * 8
			}
			if bits > maxStringTableUserDataBytes*8 {
				return nil, errStringTableUserDataTooLarge
			}
			if bits > uint64(r.bitsRemaining()) {
				return nil, errBitReadOverflow
			}
			value, err = r.readBitsAsBytes(int(bits))
			if err != nil {
				return nil, err
			}

			// Decompress the value when the entry is flagged compressed.
			if compressed {
				decodedLen, err := snappy.DecodedLen(value)
				if err != nil {
					return nil, err
				}
				if decodedLen > maxStringTableUserDataBytes {
					return nil, errStringTableUserDataTooLarge
				}
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
