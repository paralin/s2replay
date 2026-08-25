package s2replay

import (
	"io"

	"github.com/paralin/s2replay/protocol"
)

// skippedMessageKey counts packet or user messages the parser skipped.
type skippedMessageKey struct {
	userMsg   bool
	kind      int32
	decodeErr bool
}

// NextMessage returns the next decoded packet or user message. It unwraps
// DEM_Packet, DEM_SignonPacket, and DEM_FullPacket command payloads, routes
// inner ids through generated dispatch, and updates the clock from ServerInfo.
func (p *Parser) NextMessage() (*Message, error) {
	for len(p.pending) == 0 {
		cmd, err := p.Next()
		if err != nil {
			return nil, err
		}
		if err := p.queueCommandMessages(cmd); err != nil {
			return nil, err
		}
	}

	m := p.pending[0]
	copy(p.pending, p.pending[1:])
	p.pending = p.pending[:len(p.pending)-1]
	if m.err != nil {
		return nil, m.err
	}

	return m, nil
}

func (p *Parser) queueCommandMessages(cmd *Command) error {
	decoded, ok, err := decodeDemoCommand(int32(cmd.Kind), cmd.Payload)
	if err != nil || !ok {
		return err
	}
	if err := p.applyDecodedMessage(cmd.Tick, decoded.msg); err != nil {
		return err
	}

	switch msg := decoded.msg.(type) {
	case *protocol.CDemoPacket:
		return p.queuePacketMessages(cmd.Tick, msg.GetData())
	case *protocol.CDemoFullPacket:
		if packet := msg.GetPacket(); packet != nil {
			p.applyingFullPacket = true
			err := p.queuePacketMessages(cmd.Tick, packet.GetData())
			p.applyingFullPacket = false
			if err != nil {
				return err
			}
			p.seenFullPacket = true
		}
	}
	return nil
}

func (p *Parser) queuePacketMessages(tick uint32, payload []byte) error {
	r := newPacketReader(payload)
	for r.bitsRemaining() > 8 {
		kind, err := r.readUBitVar()
		if err != nil {
			return err
		}
		size, err := r.readUvarint32()
		if err != nil {
			return err
		}
		buf, err := r.readBytes(int(size))
		if err != nil {
			return err
		}

		if int32(kind) == kind145 {
			p.applyKind145(tick, buf)
			continue
		}
		decoded, ok, err := decodePacketMessage(int32(kind), buf)
		if err != nil || !ok {
			// Unknown or undecodable messages must not abort the packet:
			// later messages in the same payload are independent and often
			// carry the events downstream analysis needs.
			p.skippedMessages[skippedMessageKey{kind: int32(kind), decodeErr: err != nil}]++
			continue
		}
		p.appendMessage(tick, decoded)

		if user, ok := decoded.msg.(*protocol.CSVCMsg_UserMessage); ok {
			userDecoded, ok, err := decodeUserMessage(user.GetMsgType(), user.GetMsgData())
			if err != nil || !ok {
				p.skippedMessages[skippedMessageKey{userMsg: true, kind: int32(user.GetMsgType()), decodeErr: err != nil}]++
				continue
			}
			p.appendMessage(tick, userDecoded)
		}
	}
	return nil
}

func (p *Parser) appendMessage(tick uint32, decoded decodedMessage) {
	if err := p.applyDecodedMessage(tick, decoded.msg); err != nil {
		p.Stop()
		p.pending = append(p.pending, &Message{
			Kind:     decoded.kind,
			Name:     decoded.name,
			Tick:     tick,
			GameTime: p.clock.GameTime(),
			Payload:  decoded.msg,
			err:      err,
		})
		return
	}
	p.pending = append(p.pending, &Message{
		Kind:     decoded.kind,
		Name:     decoded.name,
		Tick:     tick,
		GameTime: p.clock.GameTime(),
		Payload:  decoded.msg,
	})
}

func (p *Parser) applyDecodedMessage(tick uint32, msg decodedProto) error {
	switch m := msg.(type) {
	case *protocol.CSVCMsg_ServerInfo:
		p.applyServerInfo(m)
	case *protocol.CDemoSendTables:
		return p.applySendTables(m)
	case *protocol.CDemoClassInfo:
		p.applyDemoClassInfo(m)
	case *protocol.CDemoStringTables:
		return p.applyDemoStringTables(tick, m)
	case *protocol.CDemoFullPacket:
		if tables := m.GetStringTable(); tables != nil {
			return p.applyDemoStringTables(tick, tables)
		}
	case *protocol.CSVCMsg_ClassInfo:
		p.applySvcClassInfo(m)
	case *protocol.CSVCMsg_CreateStringTable:
		return p.applyCreateStringTable(tick, m)
	case *protocol.CSVCMsg_UpdateStringTable:
		return p.applyUpdateStringTable(tick, m)
	case *protocol.CSVCMsg_PacketEntities:
		if err := p.applyPacketEntities(tick, m); err != nil {
			if p.firstEntityError == "" {
				p.firstEntityError = err.Error()
			}
			p.entityStateErrors[err.Error()]++
			return nil
		}
	case *protocol.CCitadelUserMsg_AbilitiesChanged:
		p.applyAbilitiesChanged(tick, m)
	case *protocol.CCitadelUserMessage_ItemPurchaseNotification:
		p.applyItemPurchaseNotification(tick, m)
	case *protocol.CCitadelUserMessage_Damage:
		p.appendDamageEvent(tick, m)
	case *protocol.CCitadelUserMsg_RecentDamageSummary:
		p.appendDamageSummaryEvent(tick, m)
	case *protocol.CCitadelUserMsg_PostMatchDetails:
		p.appendPostMatchEvent(tick, m)
	case *protocol.CCitadelUserMsg_KillStreak:
		p.appendKillStreakEvent(tick, m)
	case *protocol.CCitadelUserMsg_StaminaConsumed:
		p.appendStaminaConsumedEvent(tick, m)
	case *protocol.CCitadelUserMsg_BossKilled:
		p.appendObjectiveEvent(tick, "boss_killed", int32(m.GetObjectiveTeam()), int32(m.GetObjectiveMaskChange()), int32(m.GetEntityKilledClass()), int32(m.GetBossesRemaining()), m.GetGametime())
	case *protocol.CCitadelUserMsg_BossDamaged:
		p.appendObjectiveEvent(tick, "boss_damaged", int32(m.GetObjectiveTeam()), int32(m.GetObjectiveId()), -1, -1, 0)
	case *protocol.CCitadelUserMsg_MidBossSpawned:
		p.appendObjectiveEvent(tick, "mid_boss_spawned", -1, -1, -1, -1, 0)
	case *protocol.CCitadelUserMsg_RejuvStatus:
		p.appendObjectiveEvent(tick, "rejuv_status", int32(m.GetKillingTeam()), int32(m.GetEventType()), int32(m.GetUserTeam()), -1, 0)
	}
	return nil
}

func (p *Parser) appendKillStreakEvent(tick uint32, msg *protocol.CCitadelUserMsg_KillStreak) {
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventKillStreak,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     int32(msg.GetPlayerPawn() & uint32(entityHandleMask)),
		PlayerSlot: p.entityPlayerSlots[int32(msg.GetPlayerPawn()&uint32(entityHandleMask))],
		KillStreak: &KillStreakEvent{
			Tick:         normalizedTick(tick),
			GameTime:     p.clock.GameTime(),
			PlayerPawn:   msg.GetPlayerPawn(),
			NumKills:     msg.GetNumKills(),
			IsFirstBlood: msg.GetIsFirstBlood(),
			StreakEnded:  msg.GetStreakEnded(),
			Duration:     msg.GetDuration(),
		},
	})
}

func (p *Parser) appendStaminaConsumedEvent(tick uint32, msg *protocol.CCitadelUserMsg_StaminaConsumed) {
	// The message defaults entindex_target to -1 when attribution is absent;
	// keep that sentinel instead of masking it into a bogus handle.
	rawTarget := msg.GetEntindexTarget()
	target := int32(-1)
	slot := int32(-1)
	if rawTarget >= 0 {
		target = int32(uint32(rawTarget) & uint32(entityHandleMask))
		if mapped, ok := p.entityPlayerSlots[target]; ok {
			slot = mapped
		}
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventStaminaConsumed,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     target,
		PlayerSlot: slot,
		StaminaConsumed: &StaminaConsumedEvent{
			Tick:           normalizedTick(tick),
			GameTime:       p.clock.GameTime(),
			EntindexTarget: target,
			StaminaBefore:  msg.GetStaminaBefore(),
			StaminaAfter:   msg.GetStaminaAfter(),
			Drained:        msg.GetDrained(),
			StaminaMax:     msg.GetStaminaMax(),
		},
	})
}

func (p *Parser) appendObjectiveEvent(tick uint32, kind string, a, b, c, d int32, gameTimeF float32) {
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventObjective,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     -1,
		PlayerSlot: -1,
		Objective: &ObjectiveEvent{
			Kind:            kind,
			ObjectiveTeam:   a,
			ObjectiveID:     b,
			EntityType:      c,
			BossesRemaining: d,
			GameTimeF:       gameTimeF,
		},
	})
}

func (p *Parser) appendPostMatchEvent(tick uint32, msg *protocol.CCitadelUserMsg_PostMatchDetails) {
	details := msg.GetMatchDetails()
	if len(details) == 0 {
		return
	}
	var contents protocol.CMsgMatchMetaDataContents
	if err := contents.UnmarshalVT(details); err != nil {
		return
	}
	info := contents.GetMatchInfo()
	if info == nil {
		return
	}
	ev := Event{
		Type:       EventPostMatch,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     -1,
		PlayerSlot: -1,
		PostMatch: &PostMatchEvent{
			MatchID:      info.GetMatchId(),
			DurationS:    info.GetDurationS(),
			MatchOutcome: int32(info.GetMatchOutcome()),
			WinningTeam:  int32(info.GetWinningTeam()),
			GameMode:     int32(info.GetGameMode()),
			MatchMode:    int32(info.GetMatchMode()),
		},
	}
	for _, pl := range info.GetPlayers() {
		if pl == nil {
			continue
		}
		out := PostMatchPlayer{
			AccountID:     pl.GetAccountId(),
			PlayerSlot:    pl.GetPlayerSlot(),
			Team:          int32(pl.GetTeam()),
			Kills:         pl.GetKills(),
			Deaths:        pl.GetDeaths(),
			Assists:       pl.GetAssists(),
			NetWorth:      pl.GetNetWorth(),
			HeroID:        pl.GetHeroId(),
			LastHits:      pl.GetLastHits(),
			Denies:        pl.GetDenies(),
			AbilityPoints: pl.GetAbilityPoints(),
			Level:         pl.GetLevel(),
		}
		for _, it := range pl.GetItems() {
			if it == nil {
				continue
			}
			out.Items = append(out.Items, PostMatchPlayerItem{
				ItemID:          it.GetItemId(),
				GameTimeS:       it.GetGameTimeS(),
				SoldTimeS:       it.GetSoldTimeS(),
				UpgradeID:       it.GetUpgradeId(),
				Flags:           it.GetFlags(),
				ImbuedAbilityID: it.GetImbuedAbilityId(),
				UpgradeInfo:     it.GetUpgradeInfo(),
			})
		}
		for _, st := range pl.GetStats() {
			if st == nil {
				continue
			}
			out.Stats = append(out.Stats, PostMatchPlayerStat{
				TimeStampS:        st.GetTimeStampS(),
				NetWorth:          st.GetNetWorth(),
				Kills:             st.GetKills(),
				Deaths:            st.GetDeaths(),
				Assists:           st.GetAssists(),
				Level:             st.GetLevel(),
				PlayerDamage:      st.GetPlayerDamage(),
				PlayerDamageTaken: st.GetPlayerDamageTaken(),
				PlayerHealing:     st.GetPlayerHealing(),
				CreepDamage:       st.GetCreepDamage(),
				NeutralDamage:     st.GetNeutralDamage(),
				BossDamage:        st.GetBossDamage(),
				DamageAbsorbed:    st.GetDamageAbsorbed(),
				DamageMitigated:   st.GetDamageMitigated(),
				ShotsHit:          st.GetShotsHit(),
				ShotsMissed:       st.GetShotsMissed(),
				WeaponPower:       st.GetWeaponPower(),
				TechPower:         st.GetTechPower(),
			})
		}
		ev.PostMatch.Players = append(ev.PostMatch.Players, out)
	}
	for _, ob := range info.GetObjectives() {
		if ob == nil {
			continue
		}
		ev.PostMatch.Objectives = append(ev.PostMatch.Objectives, PostMatchObjective{
			LegacyObjectiveID:     int32(ob.GetLegacyObjectiveId()),
			TeamObjectiveID:       int32(ob.GetTeamObjectiveId()),
			Team:                  int32(ob.GetTeam()),
			DestroyedTimeS:        ob.GetDestroyedTimeS(),
			FirstDamageTimeS:      ob.GetFirstDamageTimeS(),
			CreepDamage:           ob.GetCreepDamage(),
			CreepDamageMitigated:  ob.GetCreepDamageMitigated(),
			PlayerDamage:          ob.GetPlayerDamage(),
			PlayerDamageMitigated: ob.GetPlayerDamageMitigated(),
			PlayerSpiritDamage:    ob.GetPlayerSpiritDamage(),
		})
	}
	p.pendingEvents = append(p.pendingEvents, ev)
}

func (p *Parser) appendDamageSummaryEvent(tick uint32, msg *protocol.CCitadelUserMsg_RecentDamageSummary) {
	ev := Event{
		Type:       EventDamageSummary,
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		Entity:     -1,
		PlayerSlot: msg.GetPlayerSlot(),
		DamageSummary: &DamageSummaryEvent{
			Tick:        normalizedTick(tick),
			GameTime:    p.clock.GameTime(),
			PlayerSlot:  msg.GetPlayerSlot(),
			TotalDamage: msg.GetTotalDamage(),
			LostGold:    msg.GetLostGold(),
			StartTime:   msg.GetStartTime(),
			EndTime:     msg.GetEndTime(),
		},
	}
	for _, rec := range msg.GetDamageRecords() {
		if rec == nil {
			continue
		}
		ev.DamageSummary.DamageRecords = append(ev.DamageSummary.DamageRecords, DamageSummaryRecord{
			Damage:         rec.GetDamage(),
			Hits:           rec.GetHits(),
			DamageType:     rec.GetDamageType(),
			HeroID:         rec.GetHeroId(),
			AbilityID:      rec.GetAbilityId(),
			AttackerClass:  rec.GetAttackerClass(),
			DamageAbsorbed: rec.GetDamageAbsorbed(),
			IsKillingBlow:  rec.GetIsKillingBlow(),
			VictimHeroID:   rec.GetVictimHeroId(),
			PreDamage:      rec.GetPreDamage(),
			CritDamage:     rec.GetCritDamage(),
		})
	}
	for _, rec := range msg.GetModifierRecords() {
		if rec == nil {
			continue
		}
		ev.DamageSummary.ModifierRecords = append(ev.DamageSummary.ModifierRecords, DamageSummaryModifierRecord{
			AbilityID:      rec.GetAbilityId(),
			ModifierTypeID: rec.GetModifierTypeId(),
			EntindexCaster: rec.GetEntindexCaster(),
			StartTime:      rec.GetStartTime(),
			EndTime:        rec.GetEndTime(),
			Debuff:         rec.GetDebuff(),
		})
	}
	p.pendingEvents = append(p.pendingEvents, ev)
}

// NextDamage returns the next decoded Deadlock damage event.
func (p *Parser) NextDamage() (DamageEvent, error) {
	for {
		m, err := p.NextMessage()
		if err != nil {
			return DamageEvent{}, err
		}
		if ev, ok := m.DamageEvent(); ok {
			return ev, nil
		}
	}
}

// CollectDamage reads up to limit damage events. A non-positive limit reads the
// whole demo.
func (p *Parser) CollectDamage(limit int) ([]DamageEvent, error) {
	var events []DamageEvent
	for limit <= 0 || len(events) < limit {
		ev, err := p.NextDamage()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}
