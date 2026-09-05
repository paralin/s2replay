package s2replay

import (
	"github.com/paralin/s2replay/protocol"
)

// ToProto converts the unified event to its framed binary representation.
func (e *Event) ToProto() *protocol.ReplayEvent {
	out := &protocol.ReplayEvent{
		SchemaVersion: int32(e.SchemaVersion),
		Type:          string(e.Type),
		Tick:          e.Tick,
		GameTime:      e.GameTime,
		Entity:        e.Entity,
		PlayerSlot:    e.PlayerSlot,
		OwnedItems:    e.OwnedItems,
	}
	if e.Damage != nil {
		d := e.Damage
		out.Damage = &protocol.ReplayDamage{
			Tick:                     d.Tick,
			GameTime:                 d.GameTime,
			Damage:                   d.Damage,
			PreDamage:                d.PreDamage,
			VictimHealthNew:          d.VictimHealthNew,
			VictimHealthMax:          d.VictimHealthMax,
			DamageAbsorbed:           d.DamageAbsorbed,
			Effectiveness:            d.Effectiveness,
			CritDamage:               d.CritDamage,
			Hits:                     d.Hits,
			Attacker:                 d.Attacker,
			Victim:                   d.Victim,
			Inflictor:                d.Inflictor,
			AbilityEntity:            d.AbilityEntity,
			AbilityId:                d.AbilityID,
			DamageType:               d.DamageType,
			CitadelDamageType:        d.CitadelDamageType,
			AttackingObject:          d.AttackingObject,
			VictimShieldNew:          d.VictimShieldNew,
			VictimShieldMax:          d.VictimShieldMax,
			HealthLost:               d.HealthLost,
			HitgroupId:               d.HitgroupID,
			IsSecondaryStat:          d.IsSecondaryStat,
			OriginX:                  d.OriginX,
			OriginY:                  d.OriginY,
			OriginZ:                  d.OriginZ,
			DamageDirectionX:         d.DamageDirectionX,
			DamageDirectionY:         d.DamageDirectionY,
			DamageDirectionZ:         d.DamageDirectionZ,
			ServerTick:               d.ServerTick,
			Flags:                    d.Flags,
			AttackerClass:            d.AttackerClass,
			VictimClass:              d.VictimClass,
			PreDamageDeprecated:      d.PreDamageDeprecated,
			DamageAbsorbedDeprecated: d.DamageAbsorbedDeprecated,
		}
	}
	if e.Modifier != nil {
		m := e.Modifier
		out.Modifier = &protocol.ReplayModifier{
			Tick:                     m.Tick,
			GameTime:                 m.GameTime,
			Transition:               string(m.Transition),
			TableIndex:               m.TableIndex,
			Parent:                   m.Parent,
			SerialNumber:             m.SerialNumber,
			ModifierSubclass:         m.ModifierSubclass,
			StackCount:               m.StackCount,
			MaxStackCount:            m.MaxStackCount,
			LastAppliedTime:          m.LastAppliedTime,
			Duration:                 m.Duration,
			Caster:                   m.Caster,
			Ability:                  m.Ability,
			AuraProviderSerialNumber: m.AuraProviderSerialNumber,
			AuraProviderEhandle:      m.AuraProviderEHandle,
			AbilitySubclass:          m.AbilitySubclass,
			InAuraRange:              m.InAuraRange,
			MatchedPrior:             m.MatchedPrior,
			HasSerialNumber:          m.HasSerialNumber,
			HasLastAppliedTime:       m.HasLastAppliedTime,
			HasDuration:              m.HasDuration,
		}
	}
	if e.Purchase != nil {
		pu := e.Purchase
		out.Purchase = &protocol.ReplayPurchase{
			Tick:       pu.Tick,
			GameTime:   pu.GameTime,
			PlayerSlot: pu.PlayerSlot,
			UserId:     pu.UserID,
			AbilityId:  pu.AbilityID,
			Change:     pu.Change,
			Sell:       pu.Sell,
			Quickbuy:   pu.Quickbuy,
			Source:     pu.Source,
		}
	}
	if e.EntitySample != nil {
		es := e.EntitySample
		out.EntitySample = &protocol.ReplayEntitySample{
			Tick:        es.Tick,
			GameTime:    es.GameTime,
			Entity:      es.Entity,
			ClassId:     es.ClassID,
			ClassName:   es.ClassName,
			Health:      es.Health,
			MaxHealth:   es.MaxHealth,
			Shield:      es.Shield,
			MaxShield:   es.MaxShield,
			PositionX:   es.PositionX,
			PositionY:   es.PositionY,
			PositionZ:   es.PositionZ,
			HeroId:      es.HeroID,
			Team:        es.Team,
			HasHealth:   es.HasHealth,
			HasShield:   es.HasShield,
			HasPosition: es.HasPosition,
			HasHeroId:   es.HasHeroID,
			HasTeam:     es.HasTeam,
		}
	}
	if e.DamageSummary != nil {
		ds := e.DamageSummary
		pb := &protocol.ReplayDamageSummary{
			Tick:        ds.Tick,
			GameTime:    ds.GameTime,
			PlayerSlot:  ds.PlayerSlot,
			TotalDamage: ds.TotalDamage,
			LostGold:    ds.LostGold,
			StartTime:   ds.StartTime,
			EndTime:     ds.EndTime,
		}
		for _, r := range ds.DamageRecords {
			pb.DamageRecords = append(pb.DamageRecords, &protocol.ReplayDamageSummaryRecord{
				Damage:         r.Damage,
				Hits:           r.Hits,
				DamageType:     r.DamageType,
				HeroId:         r.HeroID,
				AbilityId:      r.AbilityID,
				AttackerClass:  r.AttackerClass,
				DamageAbsorbed: r.DamageAbsorbed,
				IsKillingBlow:  r.IsKillingBlow,
				VictimHeroId:   r.VictimHeroID,
				PreDamage:      r.PreDamage,
				CritDamage:     r.CritDamage,
			})
		}
		for _, r := range ds.ModifierRecords {
			pb.ModifierRecords = append(pb.ModifierRecords, &protocol.ReplayDamageSummaryModifierRecord{
				AbilityId:      r.AbilityID,
				ModifierTypeId: r.ModifierTypeID,
				EntindexCaster: r.EntindexCaster,
				StartTime:      r.StartTime,
				EndTime:        r.EndTime,
				Debuff:         r.Debuff,
			})
		}
		out.DamageSummary = pb
	}
	if e.PostMatch != nil {
		pm := e.PostMatch
		pb := &protocol.ReplayPostMatch{
			MatchId:      pm.MatchID,
			DurationS:    pm.DurationS,
			MatchOutcome: pm.MatchOutcome,
			WinningTeam:  pm.WinningTeam,
			GameMode:     pm.GameMode,
			MatchMode:    pm.MatchMode,
		}
		for _, pl := range pm.Players {
			pp := &protocol.ReplayPostMatchPlayer{
				AccountId:     pl.AccountID,
				PlayerSlot:    pl.PlayerSlot,
				Team:          pl.Team,
				Kills:         pl.Kills,
				Deaths:        pl.Deaths,
				Assists:       pl.Assists,
				NetWorth:      pl.NetWorth,
				HeroId:        pl.HeroID,
				LastHits:      pl.LastHits,
				Denies:        pl.Denies,
				AbilityPoints: pl.AbilityPoints,
				Level:         pl.Level,
			}
			for _, it := range pl.Items {
				pp.Items = append(pp.Items, &protocol.ReplayPostMatchItem{
					ItemId:          it.ItemID,
					GameTimeS:       it.GameTimeS,
					SoldTimeS:       it.SoldTimeS,
					UpgradeId:       it.UpgradeID,
					Flags:           it.Flags,
					ImbuedAbilityId: it.ImbuedAbilityID,
					UpgradeInfo:     it.UpgradeInfo,
				})
			}
			for _, st := range pl.Stats {
				pp.Stats = append(pp.Stats, &protocol.ReplayPostMatchStat{
					TimeStampS:        st.TimeStampS,
					NetWorth:          st.NetWorth,
					Kills:             st.Kills,
					Deaths:            st.Deaths,
					Assists:           st.Assists,
					Level:             st.Level,
					LastHits:          st.LastHits,
					Denies:            st.Denies,
					PlayerDamage:      st.PlayerDamage,
					PlayerDamageTaken: st.PlayerDamageTaken,
					PlayerHealing:     st.PlayerHealing,
					CreepDamage:       st.CreepDamage,
					NeutralDamage:     st.NeutralDamage,
					BossDamage:        st.BossDamage,
					DamageAbsorbed:    st.DamageAbsorbed,
					DamageMitigated:   st.DamageMitigated,
					ShotsHit:          st.ShotsHit,
					ShotsMissed:       st.ShotsMissed,
					WeaponPower:       st.WeaponPower,
					TechPower:         st.TechPower,
				})
			}
			pb.Players = append(pb.Players, pp)
		}
		for _, ob := range pm.Objectives {
			pb.Objectives = append(pb.Objectives, &protocol.ReplayPostMatchObjective{
				LegacyObjectiveId:     ob.LegacyObjectiveID,
				TeamObjectiveId:       ob.TeamObjectiveID,
				Team:                  ob.Team,
				DestroyedTimeS:        ob.DestroyedTimeS,
				FirstDamageTimeS:      ob.FirstDamageTimeS,
				CreepDamage:           ob.CreepDamage,
				CreepDamageMitigated:  ob.CreepDamageMitigated,
				PlayerDamage:          ob.PlayerDamage,
				PlayerDamageMitigated: ob.PlayerDamageMitigated,
				PlayerSpiritDamage:    ob.PlayerSpiritDamage,
			})
		}
		out.PostMatch = pb
	}
	if e.ControllerSample != nil {
		cs := e.ControllerSample
		out.ControllerSample = &protocol.ReplayControllerSample{
			Tick:               cs.Tick,
			GameTime:           cs.GameTime,
			Entity:             cs.Entity,
			ClassId:            cs.ClassID,
			ClassName:          cs.ClassName,
			SteamId:            cs.SteamID,
			PlayerName:         cs.PlayerName,
			NetWorth:           cs.NetWorth,
			HeroDamage:         cs.HeroDamage,
			HeroHealing:        cs.HeroHealing,
			CreepGold:          cs.CreepGold,
			CreepGoldKill:      cs.CreepGoldKill,
			CreepGoldNeutral:   cs.CreepGoldNeutral,
			CreepGoldAirOrb:    cs.CreepGoldAirOrb,
			CreepGoldGroundOrb: cs.CreepGoldGroundOrb,
			CreepGoldDeny:      cs.CreepGoldDeny,
			CreepGoldSoloBonus: cs.CreepGoldSoloBonus,
		}
	}
	if e.KillStreak != nil {
		k := e.KillStreak
		out.KillStreak = &protocol.ReplayKillStreak{
			Tick:         k.Tick,
			GameTime:     k.GameTime,
			PlayerPawn:   k.PlayerPawn,
			NumKills:     k.NumKills,
			IsFirstBlood: k.IsFirstBlood,
			StreakEnded:  k.StreakEnded,
			Duration:     k.Duration,
		}
	}
	if e.Objective != nil {
		ob := e.Objective
		out.ObjectiveEvent = &protocol.ReplayObjective{
			Kind:            ob.Kind,
			ObjectiveTeam:   ob.ObjectiveTeam,
			ObjectiveId:     ob.ObjectiveID,
			KillingTeam:     ob.KillingTeam,
			EntityType:      ob.EntityType,
			BossesRemaining: ob.BossesRemaining,
			GameTimeF:       ob.GameTimeF,
		}
	}
	return out
}

// EmitProto marshals the event as a length-delimited protobuf frame appended
// to dst.
func (e *Event) EmitProto(dst []byte) ([]byte, error) {
	msg := e.ToProto()
	size := msg.SizeVT()
	var lenBuf [binaryMaxVarintLen32]byte
	n := varintEncode(lenBuf[:], uint64(size))
	dst = append(dst, lenBuf[:n]...)
	buf, err := msg.MarshalVT()
	if err != nil {
		return dst, err
	}
	return append(dst, buf...), nil
}

// binaryMaxVarintLen32 bounds a 32-bit varint.
const binaryMaxVarintLen32 = 5

// varintEncode writes v as a base-128 varint and returns the byte count.
func varintEncode(buf []byte, v uint64) int {
	i := 0
	for v >= 0x80 {
		buf[i] = byte(v) | 0x80
		v >>= 7
		i++
	}
	buf[i] = byte(v)
	return i + 1
}
