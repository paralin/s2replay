package s2replay

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

const phase4GoldenPath = "testdata/phase4_entity_samples.json"

func TestPhase4EntitySamplesGolden(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run entity decode gate: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := collectBoundedEntitySamples(p, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 8 {
		t.Fatalf(
			"entity sample: want 8 samples, got %d (classes=%d first_classes=%s serializers=%d baselines=%d entities=%d commands=create:%d update:%d leave:%d delete:%d entity_classes=%s string_tables=%s first_entity_error=%q entity_errors=%v)",
			len(samples),
			len(p.classesByID),
			firstClassSummary(p),
			len(p.serializers),
			len(p.classBaselines),
			len(p.entities),
			p.entityCreates,
			p.entityUpdates,
			p.entityLeaves,
			p.entityDeletes,
			entitySummary(p),
			stringTableSummary(p),
			p.firstEntityError,
			p.entityStateErrors,
		)
	}
	for i, sample := range samples {
		if !sample.HasHealth {
			t.Fatalf("sample %d missing health: %+v", i, sample)
		}
		if sample.MaxHealth > 0 && (sample.Health < 0 || sample.Health > sample.MaxHealth) {
			t.Fatalf("sample %d health outside max: %+v", i, sample)
		}
		if sample.Entity < 0 || sample.ClassName == "" {
			t.Fatalf("sample %d missing entity identity: %+v", i, sample)
		}
	}

	got := formatPhase4Golden(samples)
	if os.Getenv("S2REPLAY_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(phase4GoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(phase4GoldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(phase4GoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("entity golden mismatch; rerun with S2REPLAY_UPDATE_GOLDEN=1 after verifying the decoded sample")
	}
}

func TestPhase4CommandTrace(t *testing.T) {
	if os.Getenv("S2REPLAY_TRACE_COMMANDS") != "1" {
		t.Skip("set S2REPLAY_TRACE_COMMANDS=1 to trace early demo commands")
	}
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 80; i++ {
		cmd, err := p.Next()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%03d tick=%d kind=%d size=%d", i, cmd.Tick, cmd.Kind, len(cmd.Payload))
		decoded, ok, err := decodeDemoCommand(int32(cmd.Kind), cmd.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		switch msg := decoded.msg.(type) {
		case *protocol.CDemoPacket:
			applyNonEntityPacketMessages(t, p, cmd.Tick, msg.GetData())
			tracePacketEntities(t, cmd.Tick, "packet", msg.GetData())
		case *protocol.CDemoFullPacket:
			if tables := msg.GetStringTable(); tables != nil {
				traceEntityNames(t, tables)
			}
			if pkt := msg.GetPacket(); pkt != nil {
				tracePacketEntities(t, cmd.Tick, "full", pkt.GetData())
			}
		}
	}
}

func TestPhase4FirstSnapshotTrace(t *testing.T) {
	if os.Getenv("S2REPLAY_TRACE_SNAPSHOT") != "1" {
		t.Skip("set S2REPLAY_TRACE_SNAPSHOT=1 to trace the first full packet-entity snapshot")
	}
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	for {
		cmd, err := p.Next()
		if err != nil {
			t.Fatal(err)
		}
		decoded, ok, err := decodeDemoCommand(int32(cmd.Kind), cmd.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		if err := p.applyDecodedMessage(cmd.Tick, decoded.msg); err != nil {
			t.Fatal(err)
		}
		switch msg := decoded.msg.(type) {
		case *protocol.CDemoPacket:
			tracePacketEntities(t, cmd.Tick, "packet", msg.GetData())
		case *protocol.CDemoFullPacket:
			if tables := msg.GetStringTable(); tables != nil {
				p.applyDemoStringTables(tables)
			}
			packet := msg.GetPacket()
			if packet == nil {
				continue
			}
			if traceFirstSnapshotPacketEntities(t, p, cmd.Tick, packet.GetData()) {
				return
			}
		}
	}
}

func applyNonEntityPacketMessages(t *testing.T, p *Parser, tick uint32, payload []byte) {
	t.Helper()
	r := newPacketReader(payload)
	for r.bitsRemaining() > 8 {
		kind, err := r.readUBitVar()
		if err != nil {
			t.Fatal(err)
		}
		size, err := r.readUvarint32()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := r.readBytes(int(size))
		if err != nil {
			t.Fatal(err)
		}
		if int32(kind) == int32(protocol.SVC_Messages_svc_PacketEntities) {
			continue
		}
		decoded, ok, err := decodePacketMessage(int32(kind), buf)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		if err := p.applyDecodedMessage(tick, decoded.msg); err != nil {
			t.Fatal(err)
		}
	}
}

func traceEntityNames(t *testing.T, tables *protocol.CDemoStringTables) {
	for _, table := range tables.GetTables() {
		if table.GetTableName() != "EntityNames" {
			continue
		}
		limit := len(table.GetItems())
		if limit > 24 {
			limit = 24
		}
		for i := 0; i < limit; i++ {
			item := table.GetItems()[i]
			t.Logf("entity_name index=%d str=%q data=%d", i, item.GetStr(), len(item.GetData()))
		}
	}
}

func tracePacketEntities(t *testing.T, tick uint32, src string, payload []byte) {
	r := newPacketReader(payload)
	for r.bitsRemaining() > 8 {
		kind, err := r.readUBitVar()
		if err != nil {
			t.Fatal(err)
		}
		size, err := r.readUvarint32()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := r.readBytes(int(size))
		if err != nil {
			t.Fatal(err)
		}
		if int32(kind) != int32(protocol.SVC_Messages_svc_PacketEntities) {
			continue
		}
		msg := &protocol.CSVCMsg_PacketEntities{}
		if err := msg.UnmarshalVT(buf); err != nil {
			t.Fatal(err)
		}
		t.Logf(
			"packet_entities tick=%d src=%s updated=%d max=%d legacy_delta=%v update_baseline=%v pending_full=%v baseline=%d delta_from=%d entity_data=%d serialized=%d non_tx_count=%d non_tx_data=%d out_count=%d out_data=%d pvs=%d",
			tick,
			src,
			msg.GetUpdatedEntries(),
			msg.GetMaxEntries(),
			msg.GetLegacyIsDelta(),
			msg.GetUpdateBaseline(),
			msg.GetPendingFullFrame(),
			msg.GetBaseline(),
			msg.GetDeltaFrom(),
			len(msg.GetEntityData()),
			len(msg.GetSerializedEntities()),
			nonTransmittedCount(msg),
			nonTransmittedDataLen(msg),
			outOfPVSCount(msg),
			outOfPVSDataLen(msg),
			msg.GetHasPvsVisBitsDeprecated(),
		)
	}
}

func traceFirstSnapshotPacketEntities(t *testing.T, p *Parser, tick uint32, payload []byte) bool {
	t.Helper()
	r := newPacketReader(payload)
	for r.bitsRemaining() > 8 {
		kind, err := r.readUBitVar()
		if err != nil {
			t.Fatal(err)
		}
		size, err := r.readUvarint32()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := r.readBytes(int(size))
		if err != nil {
			t.Fatal(err)
		}
		decoded, ok, err := decodePacketMessage(int32(kind), buf)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		msg, ok := decoded.msg.(*protocol.CSVCMsg_PacketEntities)
		if !ok {
			if err := p.applyDecodedMessage(tick, decoded.msg); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if msg.GetLegacyIsDelta() {
			continue
		}
		traceStringTableItems(t, p, "EntityNames", 12)
		traceClassNameContains(t, p, "Camera", 12)
		traceClassNameContains(t, p, "Light", 12)
		t.Logf(
			"snapshot classes=%d class_bits=%d serializers=%d baselines=%d entities=%d updated=%d max=%d update_baseline=%v pending_full=%v baseline=%d delta_from=%d alt_baselines=%d entity_data=%d serialized=%d serialized_head=%x non_tx_count=%d non_tx_data=%d non_tx_head=%x out_count=%d out_data=%d out_head=%x pvs=%d",
			len(p.classesByID),
			p.classIDBits,
			len(p.serializers),
			len(p.classBaselines),
			len(p.entities),
			msg.GetUpdatedEntries(),
			msg.GetMaxEntries(),
			msg.GetUpdateBaseline(),
			msg.GetPendingFullFrame(),
			msg.GetBaseline(),
			msg.GetDeltaFrom(),
			len(msg.GetAlternateBaselines()),
			len(msg.GetEntityData()),
			len(msg.GetSerializedEntities()),
			firstBytes(msg.GetSerializedEntities(), 16),
			nonTransmittedCount(msg),
			nonTransmittedDataLen(msg),
			firstBytes(nonTransmittedData(msg), 16),
			outOfPVSCount(msg),
			outOfPVSDataLen(msg),
			firstBytes(outOfPVSData(msg), 16),
			msg.GetHasPvsVisBitsDeprecated(),
		)
		for i, alternate := range msg.GetAlternateBaselines() {
			if i >= 12 {
				break
			}
			t.Logf("alternate_baseline index=%d entity=%d baseline=%d", i, alternate.GetEntityIndex(), alternate.GetBaselineIndex())
		}
		traceHeaderLikeBlock(t, p, "serialized", msg.GetSerializedEntities(), 12)
		if nt := msg.GetNonTransmittedEntities(); nt != nil {
			traceOffsetBlock(t, "non_transmitted", nt.GetData(), int(nt.GetHeaderCount()), 24)
		}
		if out := msg.GetOutofpvsEntityUpdates(); out != nil {
			traceHeaderLikeBlock(t, p, "outofpvs", out.GetData(), 12)
		}
		tracePacketEntityRecords(t, p, tick, msg, 16)
		return true
	}
	return false
}

func traceClassNameContains(t *testing.T, p *Parser, needle string, limit int) {
	t.Helper()
	n := 0
	for id := int32(0); n < limit; id++ {
		class := p.classesByID[id]
		if class == nil {
			if id > 2048 {
				return
			}
			continue
		}
		if stringsContains(class.name, needle) {
			t.Logf("class_match needle=%s class=%d name=%s", needle, class.id, class.name)
			n++
		}
	}
}

func traceStringTableItems(t *testing.T, p *Parser, name string, limit int) {
	t.Helper()
	id, ok := p.stringTables.nameIndex[name]
	if !ok {
		t.Logf("string_table name=%s missing", name)
		return
	}
	table := p.stringTables.tables[id]
	if table == nil {
		t.Logf("string_table name=%s nil", name)
		return
	}
	for i := int32(0); i < int32(limit); i++ {
		item := table.items[i]
		if item == nil {
			continue
		}
		t.Logf("string_table name=%s index=%d key=%q value=%x", name, i, item.key, firstBytes(item.value, 16))
	}
}

func traceHeaderLikeBlock(t *testing.T, p *Parser, label string, buf []byte, limit int) {
	t.Helper()
	if len(buf) == 0 {
		return
	}
	r := newPacketReader(buf)
	index := int32(-1)
	for i := 0; i < limit && r.bitsRemaining() > 8; i++ {
		off, err := r.readUBitVar()
		if err != nil {
			t.Logf("%s_record i=%d offset_error=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		index += int32(off) + 1
		cmd, err := r.readBits(2)
		if err != nil {
			t.Logf("%s_record i=%d command_error=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		if cmd&1 != 0 || cmd&2 == 0 {
			t.Logf("%s_record i=%d index=%d offset=%d command=%d bits_remaining=%d", label, i, index, off, cmd, r.bitsRemaining())
			continue
		}
		classID, err := r.readBits(p.classIDBits)
		if err != nil {
			t.Logf("%s_record i=%d class_error=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		serial, err := r.readBits(17)
		if err != nil {
			t.Logf("%s_record i=%d serial_error=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		spawnGroup, err := r.readUvarint32()
		if err != nil {
			t.Logf("%s_record i=%d spawn_error=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		className := ""
		if class := p.classesByID[int32(classID)]; class != nil {
			className = class.name
		}
		t.Logf("%s_record i=%d index=%d offset=%d command=%d class=%d name=%s serial=%d spawn_group=%d bits_remaining=%d", label, i, index, off, cmd, classID, className, serial, spawnGroup, r.bitsRemaining())
	}
}

func traceOffsetBlock(t *testing.T, label string, buf []byte, count, limit int) {
	t.Helper()
	if len(buf) == 0 {
		return
	}
	r := newPacketReader(buf)
	index := int32(-1)
	for i := 0; i < count && i < limit && r.bitsRemaining() > 0; i++ {
		off, err := r.readUBitVar()
		if err != nil {
			t.Logf("%s_offset i=%d err=%v bits_remaining=%d", label, i, err, r.bitsRemaining())
			return
		}
		index += int32(off) + 1
		t.Logf("%s_offset i=%d index=%d offset=%d bits_remaining=%d", label, i, index, off, r.bitsRemaining())
	}
}

func tracePacketEntityRecords(t *testing.T, p *Parser, tick uint32, msg *protocol.CSVCMsg_PacketEntities, limit int) {
	t.Helper()
	r := newPacketReader(msg.GetEntityData())
	index := int32(-1)
	for i := 0; i < int(msg.GetUpdatedEntries()) && i < limit; i++ {
		off, err := r.readUBitVar()
		if err != nil {
			t.Fatalf("snapshot record %d offset: %v", i, err)
		}
		index += int32(off) + 1
		cmd, err := r.readBits(2)
		if err != nil {
			t.Fatalf("snapshot record %d command: %v", i, err)
		}
		t.Logf("snapshot_record i=%d tick=%d index=%d offset=%d command=%d bits_remaining=%d", i, tick, index, off, cmd, r.bitsRemaining())
		if cmd&1 != 0 {
			continue
		}
		if cmd&2 == 0 {
			e := p.entities[index]
			if e == nil {
				before := r.bitsRemaining()
				paths, err := readFieldPaths(r)
				if err != nil {
					t.Logf("snapshot_record i=%d unknown_update index=%d field_paths_error=%v bits_remaining=%d", i, index, err, r.bitsRemaining())
					return
				}
				t.Logf("snapshot_record i=%d unknown_update index=%d field_paths=%d before=%d after=%d", i, index, len(paths), before, r.bitsRemaining())
				traceUnknownUpdateCandidates(t, p, index, paths, *r, msg.GetMaxEntries(), 12)
				if len(paths) == 0 {
					continue
				}
				return
			}
			if err := e.readFields(r); err != nil {
				t.Logf("snapshot_record i=%d update_error index=%d err=%v", i, index, err)
				return
			}
			continue
		}
		classID, err := r.readBits(p.classIDBits)
		if err != nil {
			t.Fatalf("snapshot record %d class: %v", i, err)
		}
		serial, err := r.readBits(17)
		if err != nil {
			t.Fatalf("snapshot record %d serial: %v", i, err)
		}
		spawnGroup, err := r.readUvarint32()
		if err != nil {
			t.Fatalf("snapshot record %d spawn group: %v", i, err)
		}
		class := p.classesByID[int32(classID)]
		if class == nil {
			t.Logf("snapshot_record i=%d unknown_class index=%d class=%d serial=%d spawn_group=%d bits_remaining=%d", i, index, classID, serial, spawnGroup, r.bitsRemaining())
			return
		}
		e := newEntity(index, int32(serial), class)
		if baseline := p.classBaselines[int32(classID)]; len(baseline) != 0 {
			if class.name == "CConditionalCollidable" || class.name == "CDynamicProp" {
				br := newPacketReader(baseline)
				paths, err := readFieldPaths(br)
				if err != nil {
					t.Logf("snapshot_record i=%d baseline_paths_error index=%d class=%d name=%s err=%v bits_remaining=%d", i, index, classID, class.name, err, br.bitsRemaining())
					return
				}
				t.Logf("snapshot_record i=%d baseline_paths index=%d class=%d name=%s count=%d bits_remaining=%d paths=%s", i, index, classID, class.name, len(paths), br.bitsRemaining(), summarizeFieldPaths(class, paths, 32))
				if err := traceFieldValues(t, e, br, paths); err != nil {
					t.Logf("snapshot_record i=%d baseline_error index=%d class=%d name=%s err=%v bits_remaining=%d", i, index, classID, class.name, err, br.bitsRemaining())
					return
				}
			} else if err := e.readFields(newPacketReader(baseline)); err != nil {
				t.Logf("snapshot_record i=%d baseline_error index=%d class=%d name=%s err=%v", i, index, classID, class.name, err)
				return
			}
		}
		beforePaths := r.bitsRemaining()
		paths, err := readFieldPaths(r)
		if err != nil {
			t.Logf("snapshot_record i=%d create_paths_error index=%d class=%d name=%s err=%v bits_remaining=%d", i, index, classID, class.name, err, r.bitsRemaining())
			return
		}
		afterPaths := r.bitsRemaining()
		t.Logf("snapshot_record i=%d create_paths index=%d class=%d name=%s count=%d before=%d after=%d paths=%s", i, index, classID, class.name, len(paths), beforePaths, afterPaths, summarizeFieldPaths(class, paths, 16))
		if i == 4 {
			if err := traceFieldValues(t, e, r, paths); err != nil {
				t.Logf("snapshot_record i=%d create_error index=%d class=%d name=%s err=%v", i, index, classID, class.name, err)
				return
			}
		} else if err := e.readFieldValues(r, paths); err != nil {
			t.Logf("snapshot_record i=%d create_error index=%d class=%d name=%s err=%v", i, index, classID, class.name, err)
			return
		}
		p.entities[index] = e
		t.Logf("snapshot_record i=%d create index=%d class=%d name=%s serial=%d spawn_group=%d bits_remaining=%d", i, index, classID, class.name, serial, spawnGroup, r.bitsRemaining())
	}
}

func traceUnknownUpdateCandidates(t *testing.T, p *Parser, index int32, paths []fieldPath, afterPaths packetReader, maxEntries int32, limit int) {
	t.Helper()
	type candidate struct {
		classID       int32
		className     string
		nextIndex     int32
		nextOffset    uint32
		nextCommand   uint32
		bitsRemaining int
	}
	candidates := make([]candidate, 0, limit)
	for _, class := range p.classesByID {
		if class == nil || class.serializer == nil {
			continue
		}
		r := afterPaths
		e := newEntity(index, 0, class)
		if err := e.readFieldValues(&r, paths); err != nil {
			continue
		}
		off, err := r.readUBitVar()
		if err != nil {
			continue
		}
		nextIndex := index + int32(off) + 1
		cmd, err := r.readBits(2)
		if err != nil {
			continue
		}
		if nextIndex <= index || nextIndex > maxEntries {
			continue
		}
		candidates = append(candidates, candidate{
			classID:       class.id,
			className:     class.name,
			nextIndex:     nextIndex,
			nextOffset:    off,
			nextCommand:   cmd,
			bitsRemaining: r.bitsRemaining(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].nextIndex != candidates[j].nextIndex {
			return candidates[i].nextIndex < candidates[j].nextIndex
		}
		return candidates[i].classID < candidates[j].classID
	})
	if len(candidates) == 0 {
		t.Logf("unknown_update_candidates index=%d count=0", index)
		return
	}
	for i, c := range candidates {
		if i >= limit {
			break
		}
		t.Logf("unknown_update_candidate index=%d class=%d name=%s next_index=%d next_offset=%d next_command=%d bits_remaining=%d", index, c.classID, c.className, c.nextIndex, c.nextOffset, c.nextCommand, c.bitsRemaining)
	}
}

func summarizeFieldPaths(class *entityClass, paths []fieldPath, limit int) string {
	if len(paths) == 0 {
		return ""
	}
	parts := make([]string, 0, min(len(paths), limit))
	for i, fp := range paths {
		if i >= limit {
			break
		}
		f := class.fieldByPath(fp)
		detail := ""
		if f != nil {
			bits := ""
			if f.bitCount != nil {
				bits = strconv.Itoa(int(*f.bitCount))
			}
			qdetail := ""
			if f.fieldType.baseType == "CNetworkedQuantizedFloat" || f.fieldType.baseType == "float32" {
				q := newQuantizedFloatDecoder(f.bitCount, f.encodeFlags, f.lowValue, f.highValue)
				qdetail = ",flags=" + int32PtrString(f.encodeFlags) +
					",low=" + float32PtrString(f.lowValue) +
					",high=" + float32PtrString(f.highValue) +
					",qflags=" + strconv.Itoa(int(q.flags)) +
					",qbits=" + strconv.Itoa(int(q.bitCount))
			}
			detail = "(" + f.varType + ",enc=" + f.encoder + ",bits=" + bits + ",model=" + strconv.Itoa(f.model) + qdetail + ")"
		}
		parts = append(parts, fp.String()+":"+class.fieldName(fp)+detail)
	}
	if len(paths) > limit {
		parts = append(parts, "...")
	}
	return strings.Join(parts, ",")
}

func int32PtrString(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}

func float32PtrString(v *float32) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*v), 'g', -1, 32)
}

func traceFieldValues(t *testing.T, e *Entity, r *packetReader, paths []fieldPath) error {
	t.Helper()
	for _, fp := range paths {
		d := e.class.decoder(fp)
		if d == nil {
			return errUnknownFieldPath
		}
		before := r.bitsRemaining()
		v, err := d(r)
		if err != nil {
			return err
		}
		after := r.bitsRemaining()
		e.state.set(fp, v)
		t.Logf("field_value entity=%d class=%s path=%s name=%s bits=%d before=%d after=%d value=%v", e.index, e.class.name, fp.String(), e.class.fieldName(fp), before-after, before, after, v)
	}
	return nil
}

func (e *Entity) readFieldValues(r *packetReader, paths []fieldPath) error {
	for _, fp := range paths {
		d := e.class.decoder(fp)
		if d == nil {
			return errUnknownFieldPath
		}
		v, err := d(r)
		if err != nil {
			return err
		}
		e.state.set(fp, v)
	}
	return nil
}

func nonTransmittedCount(msg *protocol.CSVCMsg_PacketEntities) int32 {
	if msg.GetNonTransmittedEntities() == nil {
		return 0
	}
	return msg.GetNonTransmittedEntities().GetHeaderCount()
}

func nonTransmittedDataLen(msg *protocol.CSVCMsg_PacketEntities) int {
	if msg.GetNonTransmittedEntities() == nil {
		return 0
	}
	return len(msg.GetNonTransmittedEntities().GetData())
}

func nonTransmittedData(msg *protocol.CSVCMsg_PacketEntities) []byte {
	if msg.GetNonTransmittedEntities() == nil {
		return nil
	}
	return msg.GetNonTransmittedEntities().GetData()
}

func outOfPVSCount(msg *protocol.CSVCMsg_PacketEntities) int32 {
	if msg.GetOutofpvsEntityUpdates() == nil {
		return 0
	}
	return msg.GetOutofpvsEntityUpdates().GetCount()
}

func outOfPVSDataLen(msg *protocol.CSVCMsg_PacketEntities) int {
	if msg.GetOutofpvsEntityUpdates() == nil {
		return 0
	}
	return len(msg.GetOutofpvsEntityUpdates().GetData())
}

func outOfPVSData(msg *protocol.CSVCMsg_PacketEntities) []byte {
	if msg.GetOutofpvsEntityUpdates() == nil {
		return nil
	}
	return msg.GetOutofpvsEntityUpdates().GetData()
}

func firstBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}

func firstClassSummary(p *Parser) string {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i := 0; i < 12; i++ {
		if i != 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(strconv.Itoa(i))
		buf.WriteByte(':')
		if class := p.classesByID[int32(i)]; class != nil {
			buf.WriteString(class.name)
		} else {
			buf.WriteByte('?')
		}
	}
	buf.WriteByte(']')
	return buf.String()
}

func entitySummary(p *Parser) string {
	var buf bytes.Buffer
	buf.WriteByte('[')
	first := true
	for _, entity := range p.entities {
		if entity == nil || entity.class == nil {
			continue
		}
		if !first {
			buf.WriteString(", ")
		}
		first = false
		buf.WriteString(strconv.Itoa(int(entity.index)))
		buf.WriteByte(':')
		buf.WriteString(entity.class.name)
	}
	buf.WriteByte(']')
	return buf.String()
}

func stringTableSummary(p *Parser) string {
	var buf bytes.Buffer
	buf.WriteByte('[')
	first := true
	for _, table := range p.stringTables.tables {
		if !first {
			buf.WriteString(", ")
		}
		first = false
		buf.WriteString(table.name)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(len(table.items)))
	}
	buf.WriteByte(']')
	return buf.String()
}

func collectBoundedEntitySamples(p *Parser, limit int) ([]EntitySample, error) {
	samples := make([]EntitySample, 0, limit)
	for len(samples) < limit {
		sample, err := p.NextEntitySample()
		if err == io.EOF {
			return samples, nil
		}
		if err != nil {
			return samples, err
		}
		if sample.HasHealth && sample.MaxHealth > 0 && sample.Health >= 0 && sample.Health <= sample.MaxHealth {
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func formatPhase4Golden(samples []EntitySample) []byte {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	buf.WriteString("  \"entity_samples\": [\n")
	for i, sample := range samples {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeEntitySampleJSON(&buf, sample)
	}
	buf.WriteString("\n  ]\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeEntitySampleJSON(buf *bytes.Buffer, sample EntitySample) {
	buf.WriteString("    {\n")
	writeJSONUint(buf, "tick", uint64(sample.Tick), true)
	writeJSONFloat64(buf, "game_time", sample.GameTime, true)
	writeJSONInt(buf, "entity", int64(sample.Entity), true)
	writeJSONInt(buf, "class_id", int64(sample.ClassID), true)
	writeJSONString(buf, "class_name", sample.ClassName, true)
	writeJSONFloat32(buf, "health", sample.Health, true)
	writeJSONFloat32(buf, "max_health", sample.MaxHealth, true)
	writeJSONFloat32(buf, "shield", sample.Shield, true)
	writeJSONFloat32(buf, "max_shield", sample.MaxShield, true)
	writeJSONFloat32(buf, "position_x", sample.PositionX, true)
	writeJSONFloat32(buf, "position_y", sample.PositionY, true)
	writeJSONFloat32(buf, "position_z", sample.PositionZ, true)
	writeJSONBool(buf, "has_health", sample.HasHealth, true)
	writeJSONBool(buf, "has_shield", sample.HasShield, true)
	writeJSONBool(buf, "has_position", sample.HasPosition, false)
	buf.WriteString("    }")
}

func writeJSONString(buf *bytes.Buffer, name string, v string, comma bool) {
	writeJSONName(buf, name)
	buf.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\', '"':
			buf.WriteByte('\\')
			buf.WriteByte(v[i])
		default:
			buf.WriteByte(v[i])
		}
	}
	buf.WriteByte('"')
	writeJSONLineEnd(buf, comma)
}
