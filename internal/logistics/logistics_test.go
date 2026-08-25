package logistics

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// straightRoad returns Road buildings filling every tile from x=fromX to
// x=toX-1 at row y, connecting whatever sits at fromX and toX.
func straightRoad(fromX, toX, y int) []*building.Building {
	var roads []*building.Building
	for x := fromX; x < toX; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	return roads
}

func TestController_CollectsFromProducerToWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0} // (0,0)-(1,1)
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}           // (5,0)-(6,1)
	farm.AddOutput(resource.Wheat, 5)

	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 500 {
		c.Tick(buildings, stock)
		if stock.Amount(resource.Wheat) > 0 {
			break
		}
	}

	if got := stock.Amount(resource.Wheat); got != 5 {
		t.Fatalf("warehouse Wheat = %d, want 5 (serf should have collected the farm's output)", got)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != 0 {
		t.Fatalf("farm OutputBuffer[Wheat] = %d, want 0 (collected)", got)
	}
}

func TestController_SuppliesConsumerFromWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 0}

	buildings := append([]*building.Building{warehouse, mill}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Wheat, 20)

	for range 500 {
		c.Tick(buildings, stock)
		if mill.InputBuffer[resource.Wheat] > 0 {
			break
		}
	}

	if got := mill.InputBuffer[resource.Wheat]; got != 5 {
		t.Fatalf("mill InputBuffer[Wheat] = %d, want 5 (serf should have supplied it)", got)
	}
	if got := stock.Amount(resource.Wheat); got != 15 {
		t.Fatalf("warehouse Wheat = %d, want 15 (5 handed off to the mill)", got)
	}
}

func TestController_DisconnectedBuildingIsNeverServiced(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 5)

	// No road at all between them.
	buildings := []*building.Building{warehouse, farm}

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 200 {
		c.Tick(buildings, stock)
	}

	if got := stock.Amount(resource.Wheat); got != 0 {
		t.Fatalf("warehouse Wheat = %d, want 0 (farm has no road connection)", got)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != 5 {
		t.Fatalf("farm OutputBuffer[Wheat] = %d, want 5 (still sitting there, uncollected)", got)
	}
}
