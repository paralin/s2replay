package main

import "github.com/paralin/s2replay/protocol"

// classBaselineTable maps class keys to serialized default field values.
const classBaselineTable = "instancebaseline"

// baselineOrder holds the class keys at their fixed indices in the output demo.
// An update to an occupied client slot changes its data, not its key.
type baselineOrder []string

// align puts the snapshot's baseline data in the output table's order. It
// reports whether the source used a different order. This experiment supports
// matching key sets; it cannot remove a key once the client has seen it.
func (order *baselineOrder) align(snapshot *protocol.CDemoStringTables) bool {
	// The first snapshot establishes the order without altering the cut.
	for i, table := range snapshot.GetTables() {
		if table.GetTableName() != classBaselineTable {
			continue
		}
		items := table.GetItems()
		if *order == nil {
			*order = make(baselineOrder, len(items))
			for j, item := range items {
				(*order)[j] = item.GetStr()
			}
			return false
		}

		// Match class values by key, not by the source's slot number.
		if len(items) != len(*order) {
			panic("splice baseline snapshots have different key sets")
		}
		byKey := make(map[string]*protocol.CDemoStringTablesItemsT, len(items))
		for _, item := range items {
			byKey[item.GetStr()] = item
		}
		aligned := make([]*protocol.CDemoStringTablesItemsT, len(items))
		changed := false
		for j, key := range *order {
			item, ok := byKey[key]
			if !ok {
				panic("splice baseline snapshots have different key sets")
			}
			aligned[j] = item
			changed = changed || key != items[j].GetStr()
		}

		// Don't reorder the reusable source tables.
		if changed {
			clone := *table
			clone.Items = aligned
			snapshot.Tables[i] = &clone
		}
		return changed
	}
	return false
}

// requireNoBaselineUpdate refuses incremental source indices after a remap.
// The two keyframe-aligned Prop Hunt control ranges contain none.
func requireNoBaselineUpdate(data []byte, tableID int32) {
	eachMsg(data, func(kind uint32, body []byte) {
		if kind != uint32(protocol.SVC_Messages_svc_UpdateStringTable) {
			return
		}
		var update protocol.CSVCMsg_UpdateStringTable
		must(update.UnmarshalVT(body))
		if update.GetTableId() == tableID {
			panic("splice baseline update needs source index remapping")
		}
	})
}
