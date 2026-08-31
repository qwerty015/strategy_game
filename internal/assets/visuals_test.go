package assets

import (
	"strings"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func TestBuildingVisualsCoverEveryConstructedKind(t *testing.T) {
	kinds := []building.Kind{
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
	}
	for _, kind := range kinds {
		visual, ok := BuildingVisualFor(kind)
		if !ok {
			t.Fatalf("constructed kind %v has no visual definition", kind)
		}
		for _, stage := range []building.ConstructionStage{
			building.ConstructionFoundation,
			building.ConstructionWaitingMaterials,
			building.ConstructionFinishing,
		} {
			if visual.Construction[stage].Site.Image == nil {
				t.Fatalf("kind %v stage %v has no construction-site art", kind, stage)
			}
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
func TestFarmUsesDedicatedConstructionLayersWhenTheyAreAvailable(t *testing.T) {
	visual, ok := BuildingVisualFor(building.Farm)
	if !ok {
		t.Fatal("farm visual definition is missing")
	}
	for _, stage := range []building.ConstructionStage{
		building.ConstructionFoundation,
		building.ConstructionWaitingMaterials,
		building.ConstructionFinishing,
	} {
		layer := visual.Construction[stage].Site
		if layer.Image == nil || layer.Image.Bounds().Dx() <= TileSize {
			t.Fatalf("farm stage %v did not load its dedicated native layer", stage)
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
	} {
		visual, ok := BuildingVisualFor(kind)
		if !ok || visual.Body.TilesTall < 1.8 {
			t.Fatalf("building %v does not have the expected large visual extent", kind)
		}
	}
}
