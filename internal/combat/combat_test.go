package combat

import "testing"

func TestApplyDamageClampsAtZero(t *testing.T) {
	if got := ApplyDamage(5, DamagePerHit); got != 0 {
		t.Fatalf("ApplyDamage(5, %d) = %d, want 0 (clamped)", DamagePerHit, got)
	}
	if got := ApplyDamage(MaxHP, DamagePerHit); got != MaxHP-DamagePerHit {
		t.Fatalf("ApplyDamage(%d, %d) = %d, want %d", MaxHP, DamagePerHit, got, MaxHP-DamagePerHit)
	}
}

func TestTenHitsDestroyAFullHealthBuilding(t *testing.T) {
	hp := MaxHP
	for i := range 10 {
		if IsDestroyed(hp) {
			t.Fatalf("destroyed after only %d hits, want exactly 10", i)
		}
		hp = ApplyDamage(hp, DamagePerHit)
	}
	if !IsDestroyed(hp) {
		t.Fatalf("hp = %d after 10 hits, want destroyed", hp)
	}
}

func TestRepairClampsAtMaxHP(t *testing.T) {
	if got := Repair(MaxHP-5, 50); got != MaxHP {
		t.Fatalf("Repair(%d, 50) = %d, want %d (clamped)", MaxHP-5, got, MaxHP)
	}
}

func TestIsDestroyedAtZeroOrBelow(t *testing.T) {
	if !IsDestroyed(0) {
		t.Fatal("IsDestroyed(0) = false, want true")
	}
	if IsDestroyed(1) {
		t.Fatal("IsDestroyed(1) = true, want false")
	}
}
