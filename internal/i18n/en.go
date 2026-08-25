package i18n

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func init() {
	register(EN, Catalog{
		WindowTitle: "Town Builder",

		Population: "Population",
		ResourceName: map[resource.Type]string{
			resource.Wheat: "Wheat",
			resource.Flour: "Flour",
			resource.Bread: "Bread",
		},
		BuildingName: map[building.Kind]string{
			building.Farm:      "Farm",
			building.Mill:      "Mill",
			building.Bakery:    "Bakery",
			building.Warehouse: "Warehouse",
			building.Road:      "Road",
		},

		Help:                  "Arrows: pan | 1-4: select building | Click: place | S: save | L: load | Esc: quit",
		CantBuildHere:         "Can't build there",
		SaveFailedPrefix:      "Save failed: ",
		LoadFailedPrefix:      "Load failed: ",
		Saved:                 "Saved.",
		Loaded:                "Loaded.",
		LoadFailedNoWarehouse: "Load failed: save has no warehouse",
	})
}
