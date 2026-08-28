package builder

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// TestBuilder_EatsAtNearestReachableTavern mirrors quarry/lumberjack's test
// of the same shape: a hungry, idle builder (no construction site to work
// on) must prioritize walking to the nearest reachable, stocked Tavern over
// staying idle, and must actually eat there -- taking food from that
// Tavern's InputBuffer and resetting hunger to 0 -- not just walk there and
// stand around.
func TestBuilder_EatsAtNearestReachableTavern(t *testing.T) {
	grid := world.NewGrid(15, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 12, Y: 0}
	tavernNear := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := []*building.Building{warehouse, tavernFar, tavernNear}
	controller := NewController()
	b := controller.Hire(warehouse)

	var ate bool
	for range HungerInterval + 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if b.hungerTick == 0 {
			ate = true
			break
		}
	}

	if !ate {
		t.Fatal("builder never ate in time")
	}
	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (builder should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

// TestBuilder_EatsWhileWaitingForMaterialsInsteadOfStarving is a regression
// guard for a real bug the user reported ("погибают строители ожидая
// ресурсы"): StateWaitingMaterials had no upper bound (a serf can take a
// very long time to reach a distant, scarce-material site) and never
// checked hunger -- only StateIdle did -- so a builder stuck waiting simply
// starved to death with zero chance to eat, no matter how close a stocked
// Tavern was. Runs well past hunger.MaxTicks with materials deliberately
// never delivered, and the builder must still be alive, having eaten more
// than once, and must still complete the build once materials do arrive --
// proving the wait was actually resumed at the same site (not lost to a
// fresh, from-scratch startSiteJob that would redo the foundation).
func TestBuilder_EatsWhileWaitingForMaterialsInsteadOfStarving(t *testing.T) {
	grid := world.NewGrid(15, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	site := building.NewConstructionSite(building.Mill, 8, 0)
	buildings := []*building.Building{warehouse, tavern, site}
	controller := NewController()
	b := controller.Hire(warehouse)
	if b == nil {
		t.Fatal("Hire() returned nil")
	}

	meals := 0
	lastHunger := 0
	for range hunger.MaxTicks*2 + 100 {
		tavern.AddInput(resource.Bread, 1) // keep the Tavern stocked; food is never the constraint here
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if len(controller.Builders) == 0 {
			t.Fatalf("builder died despite a stocked, reachable Tavern (last hunger tick observed: %d)", lastHunger)
		}
		if b.hungerTick == 0 && lastHunger > 0 {
			meals++
		}
		lastHunger = b.hungerTick
	}
	if meals < 2 {
		t.Fatalf("builder ate %d times over %d ticks, want at least 2 (should cycle to the Tavern repeatedly while stuck waiting)", meals, hunger.MaxTicks*2+100)
	}
	// The loop above may have ended mid-trip to/from the Tavern; give it a
	// short, bounded window to settle back into StateWaitingMaterials
	// before asserting on it.
	settled := false
	for range 100 {
		tavern.AddInput(resource.Bread, 1)
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if b.State() == StateWaitingMaterials {
			settled = true
			break
		}
	}
	if !settled {
		t.Fatalf("builder never settled back into StateWaitingMaterials, last state = %v", b.State())
	}

	// Now actually deliver the materials and confirm construction still
	// completes normally -- proving the site's foundation progress was
	// never silently redone or lost across all those meal trips.
	site.AddConstructionMaterial(resource.Plank, building.Types[building.Mill].PlankCost)
	site.AddConstructionMaterial(resource.StoneBlock, building.Types[building.Mill].StoneCost)

	var completed *Event
	for range building.Types[building.Mill].ConstructionBuildTicks + 5 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
			e := event
			completed = &e
		}
		if completed != nil {
			break
		}
	}
	if completed == nil {
		t.Fatal("builder never finished construction after materials arrived")
	}
	if completed.Kind != ConstructionComplete || completed.Building != site {
		t.Fatalf("event = %#v, want ConstructionComplete for the site", completed)
	}
}

// TestBuilderCompletesConstructionInTwoPhases covers the core two-phase
// flow the user asked for: the builder starts working the instant he
// arrives (foundation, no materials needed yet), then waits once the
// foundation is done until materials are delivered, then finishes and
// fires ConstructionComplete.
func TestBuilderCompletesConstructionInTwoPhases(t *testing.T) {
	grid := world.NewGrid(12, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Mill, 5, 0)
	buildings := []*building.Building{warehouse, site}
	controller := NewController()
	b := controller.Hire(warehouse)
	if b == nil {
		t.Fatal("Hire() returned nil")
	}

	// Run through the walk to the site plus the whole foundation phase (a
	// generous budget covers both). No materials are on site, so the
	// builder must be waiting, not finishing, the instant it ends.
	for range building.Types[building.Mill].ConstructionFoundationTicks + 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if b.State() == StateWaitingMaterials {
			break
		}
	}
	if b.State() != StateWaitingMaterials {
		t.Fatalf("state after the foundation phase = %v, want StateWaitingMaterials (no materials delivered yet)", b.State())
	}
	if site.ConstructionStage != building.ConstructionWaitingMaterials {
		t.Fatalf("site ConstructionStage = %v, want ConstructionWaitingMaterials", site.ConstructionStage)
	}

	// Deliver the required materials (as a serf normally would) and confirm
	// the builder resumes and eventually finishes.
	site.AddConstructionMaterial(resource.Plank, building.Types[building.Mill].PlankCost)
	site.AddConstructionMaterial(resource.StoneBlock, building.Types[building.Mill].StoneCost)

	var completed *Event
	for range building.Types[building.Mill].ConstructionBuildTicks + 5 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
			e := event
			completed = &e
		}
		if completed != nil {
			break
		}
	}
	if completed == nil {
		t.Fatal("builder never finished construction after materials arrived")
	}
	if completed.Kind != ConstructionComplete || completed.Building != site {
		t.Fatalf("event = %#v, want ConstructionComplete for the site", completed)
	}
	if site.ConstructionStage != building.ConstructionNone {
		t.Fatalf("site ConstructionStage after completion = %v, want ConstructionNone", site.ConstructionStage)
	}
	if got := site.InputBuffer[resource.Plank]; got != 0 {
		t.Fatalf("site InputBuffer[Plank] after completion = %d, want 0 (consumed, not left behind)", got)
	}
}

// TestBuilderSkipsWaitingWhenMaterialsAlreadyDelivered covers the other
// path through the two-phase gate: if a serf already delivered everything
// before the foundation phase even ends, the builder must go straight to
// finishing instead of idling in StateWaitingMaterials for one extra tick.
func TestBuilderSkipsWaitingWhenMaterialsAlreadyDelivered(t *testing.T) {
	grid := world.NewGrid(12, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Road, 5, 0)
	site.AddConstructionMaterial(resource.StoneBlock, building.Types[building.Road].StoneCost)
	buildings := []*building.Building{warehouse, site}
	controller := NewController()
	controller.Hire(warehouse)

	sawWaiting := false
	sawFinishing := false
	for range building.Types[building.Road].ConstructionFoundationTicks + building.Types[building.Road].ConstructionBuildTicks + 30 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if len(controller.Builders) == 0 {
			break
		}
		switch controller.Builders[0].State() {
		case StateWaitingMaterials:
			sawWaiting = true
		case StateFinishing:
			sawFinishing = true
		}
		if site.ConstructionStage == building.ConstructionNone {
			break
		}
	}
	if sawWaiting {
		t.Fatal("builder entered StateWaitingMaterials even though materials were already delivered before the foundation finished")
	}
	if !sawFinishing {
		t.Fatal("builder never reached StateFinishing")
	}
}

// TestOnlyOneBuilderClaimsASite covers "не даём второму строителю тот же
// объект": a second builder must go looking for a different site rather
// than pile onto one already claimed, unlike a stone deposit where sharing
// is fine.
func TestOnlyOneBuilderClaimsASite(t *testing.T) {
	grid := world.NewGrid(12, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Mill, 5, 0)
	buildings := []*building.Building{warehouse, site}
	controller := NewController()
	controller.Hire(warehouse)
	controller.Hire(warehouse)

	for range 10 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	claimants := 0
	for _, b := range controller.Builders {
		if b.TargetSite() == site {
			claimants++
		}
	}
	if claimants != 1 {
		t.Fatalf("builders targeting the one site = %d, want 1", claimants)
	}
}

// TestController_CancelRouteToResetsBuilderWithoutDanglingPointer mirrors
// the equivalent lumberjack/quarry test: cancelling a site (or deleting a
// Tavern a builder is mid-walk to eat at) must not leave a dangling
// pointer to a building no longer in the world.
func TestController_CancelRouteToResetsBuilderWithoutDanglingPointer(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Mill, 5, 0)

	controller := NewController()
	b := controller.Hire(warehouse)
	b.state = StateFoundation
	b.target = site

	controller.CancelRouteTo(site)

	if b.state != StateIdle {
		t.Fatalf("builder state = %v, want StateIdle", b.state)
	}
	if b.target != nil {
		t.Fatal("b.target is still set after CancelRouteTo -- dangling pointer to the cancelled site")
	}
}

// TestController_SurvivesManyIdleTicksWithNoWork mirrors the same guard
// used for every other free-roaming worker controller: a builder with
// nothing to build must not silently vanish from the roster while idle.
func TestController_SurvivesManyIdleTicksWithNoWork(t *testing.T) {
	grid := world.NewGrid(8, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse} // nothing under construction
	controller := NewController()
	controller.Hire(warehouse)

	for range 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	if got := len(controller.Builders); got != 1 {
		t.Fatalf("builders after 50 idle ticks = %d, want 1 (worker must not vanish while merely idle)", got)
	}
}
