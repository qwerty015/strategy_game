package assets

import (
	"strings"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// TestBuildingVisualsCoverEveryConstructedKind checks every constructible
// kind that goes through render.drawConstructionSite's Body-fade path (see
// its doc comment) has a real Body sprite to fade -- per the user's
// request to fade a building's (or road's) own real sprite up from
// translucent instead of keeping separate foundation/waiting/finishing art
// per kind. StoneWall/Gate are deliberately excluded: they have no single
// static Body at all (their look depends on live neighbour segments, see
// drawWalls/wallPreviewArt), so BuildingVisualFor has no entry for them.
func TestBuildingVisualsCoverEveryConstructedKind(t *testing.T) {
	kindsWithABody := []building.Kind{
		building.Road,
		building.Farm,
		building.Mill,
		building.Bakery,
		building.Warehouse,
		building.Tavern,
		building.LumberjackHut,
		building.Winery,
		building.FisherHut,
		building.PigFarm,
		building.MeatWorkshop,
		building.CarpentryWorkshop,
		building.QuarryHut,
		building.MinerHut,
		building.Smeltery,
		building.WatchTower,
		building.Barracks,
		building.Armory,
	}
	for _, kind := range kindsWithABody {
		visual, ok := BuildingVisualFor(kind)
		if !ok || visual.Body.Image == nil {
			t.Fatalf("kind %v has no Body sprite to fade in during construction", kind)
		}
	}
}

func TestResourceVisualsCoverEveryEconomyResource(t *testing.T) {
	for _, kind := range resource.AllTypes() {
		visual, ok := ResourceVisualFor(kind)
		if !ok {
			t.Fatalf("resource %v has no visual definition", kind)
		}
		for label, path := range map[string]string{
			"icon":  visual.Icon,
			"carry": visual.Carry,
			"stack": visual.Stack,
		} {
			if !strings.HasPrefix(path, "resources/") || !strings.HasSuffix(path, ".png") {
				t.Fatalf("resource %v has invalid %s path %q", kind, label, path)
			}
		}
		if ResourceIcon(kind) == nil || ResourceCarry(kind) == nil || ResourceStack(kind) == nil {
			t.Fatalf("resource %v is missing decoded icon/carry/stack art", kind)
		}
	}
}
func TestProductionBuildingsUseLargeVisualFootprints(t *testing.T) {
	for _, kind := range []building.Kind{
		building.Farm,
		building.Mill,
		building.Bakery,
		building.Warehouse,
		building.Tavern,
		building.LumberjackHut,
		building.Winery,
		building.FisherHut,
		building.PigFarm,
		building.MeatWorkshop,
		building.CarpentryWorkshop,
		building.QuarryHut,
		building.MinerHut,
		building.Smeltery,
		building.WatchTower,
		building.Barracks,
		building.Armory,
	} {
		visual, ok := BuildingVisualFor(kind)
		if !ok || visual.Body.TilesTall < 1.8 {
			t.Fatalf("building %v does not have the expected large visual extent", kind)
		}
	}
}
