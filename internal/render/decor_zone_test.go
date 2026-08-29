package render

import "testing"

// TestDecorZoneKindAt_AllThreeKindsOccur is a basic sanity check that the
// zone roll actually produces all three outcomes over a large enough area
// -- if the percentage thresholds in decorZoneKindAt were ever typo'd
// into an unreachable range, this would catch a zone kind silently never
// appearing at all.
func TestDecorZoneKindAt_AllThreeKindsOccur(t *testing.T) {
	seen := map[decorZoneKind]bool{}
	for zx := range 40 {
		for zy := range 40 {
			seen[decorZoneKindAt(zx*decorZoneSize, zy*decorZoneSize)] = true
		}
	}
	for _, want := range [...]decorZoneKind{zoneNone, zoneMeadow, zoneRocky} {
		if !seen[want] {
			t.Errorf("decorZoneKindAt never produced %v over a 40x40 zone-cell sample", want)
		}
	}
}

// TestGrassDecorPick_ZonesAreDenserAndBiased is a regression guard for
// the whole point of a decorative zone: a screenshot can't reliably
// distinguish "3x denser" by eye from ordinary per-tile scatter, so this
// checks the actual numbers over a large sample -- both zone kinds must
// noticeably out-spawn the unzoned baseline, and each must lean toward
// its own flavour of decoration (flowers/bushes for a meadow, rocks/
// stumps for a rocky patch) rather than an even split across all four
// variants.
func TestGrassDecorPick_ZonesAreDenserAndBiased(t *testing.T) {
	type stats struct {
		tiles, hits int
		variants    [4]int
	}
	byZone := map[decorZoneKind]*stats{zoneNone: {}, zoneMeadow: {}, zoneRocky: {}}

	const span = 400 // tiles per side -- several zone cells of each kind
	for tx := range span {
		for ty := range span {
			s := byZone[decorZoneKindAt(tx, ty)]
			s.tiles++
			if variant, ok := grassDecorPick(tx, ty); ok {
				s.hits++
				s.variants[variant]++
			}
		}
	}

	densityOf := func(s *stats) float64 { return float64(s.hits) / float64(s.tiles) }
	baseline := densityOf(byZone[zoneNone])
	for _, kind := range [...]decorZoneKind{zoneMeadow, zoneRocky} {
		s := byZone[kind]
		if got := densityOf(s); got < baseline*2 {
			t.Errorf("%v density = %.4f, want at least 2x the unzoned baseline (%.4f)", kind, got, baseline)
		}
	}

	meadow := byZone[zoneMeadow]
	if flowersAndBushes := meadow.variants[2] + meadow.variants[3]; float64(flowersAndBushes) < float64(meadow.hits)*0.8 {
		t.Errorf("meadow zone: flower+bush = %d of %d decorated tiles, want at least 80%%", flowersAndBushes, meadow.hits)
	}

	rocky := byZone[zoneRocky]
	if rocksAndStumps := rocky.variants[0] + rocky.variants[1]; float64(rocksAndStumps) < float64(rocky.hits)*0.8 {
		t.Errorf("rocky zone: rock+stump = %d of %d decorated tiles, want at least 80%%", rocksAndStumps, rocky.hits)
	}
}
