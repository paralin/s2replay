package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

// TestTimelineCastsAndFacing feeds a hero's facing, its ability uses and a
// trooper's position to the builder: the hero's yaw is its facing, movement
// and melee uses are left out, and a use by an unknown owner is dropped.
func TestTimelineCastsAndFacing(t *testing.T) {
	// Introduce a hero pawn that faces 90 degrees.
	b := newTimelineBuilder(nil)
	b.acceptPawn(&s2replay.EntitySample{
		ClassName: "CCitadelPlayerPawn", Entity: 7, HasOwnerEntity: true, OwnerEntity: 3, HasHeroID: true, HeroID: 17,
		HasFacing: true, FacingY: 90,
	})

	// Use two abilities, a jump and an unowned ability.
	b.acceptAbilityCast(10, &s2replay.AbilityCastEvent{OwnerEntity: 7, ClassName: "CCitadel_Ability_Fireball", SubclassID: 55})
	b.acceptAbilityCast(11, &s2replay.AbilityCastEvent{OwnerEntity: 7, ClassName: "CCitadel_Ability_Jump"})
	b.acceptAbilityCast(12, &s2replay.AbilityCastEvent{OwnerEntity: 7, ClassName: "CCitadel_Ability_MeleeAttack"})
	b.acceptAbilityCast(13, &s2replay.AbilityCastEvent{OwnerEntity: 99, ClassName: "CCitadel_Ability_Fireball"})
	b.acceptTrooper(14, &s2replay.EntitySample{Entity: 200, EntitySerial: 4, Team: 2, HasPosition: true, PositionX: 5, PositionY: 6})

	// Check what the builder kept.
	if yaw := b.byPawn[7].frame.yaw; yaw != 90 {
		t.Fatalf("yaw = %v, want the facing 90", yaw)
	}
	if casts := b.abilityCasts; len(casts) != 1 || casts[0].cast.GetClassName() != "CCitadel_Ability_Fireball" || casts[0].cast.GetAbilityId() != 55 || casts[0].cast.GetSeconds() != 10 {
		t.Fatalf("ability casts = %v, want the one fireball", casts)
	}
	if tr := b.out.GetTroopers(); len(tr) != 1 || tr[0].GetEntity() != 200 || tr[0].GetX() != 5 || tr[0].GetTeam() != 2 {
		t.Fatalf("troopers = %v", tr)
	}
}
