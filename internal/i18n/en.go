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
			building.Tavern:    "Tavern",
		},

		BuildMenuTitle: "Build",
		InspectorTitle: "Inspector",
		InspectorHint:  "Select a building or unit",
		SpeedTitle:     "Speed",
		SpeedPaused:    "Pause",
		SpeedHalf:      "0.5x",
		SpeedNormal:    "1x",
		SpeedDouble:    "2x",
		SpeedQuadruple: "4x",

		StateLabel:            "State",
		CargoLabel:            "Cargo",
		RouteLabel:            "Route",
		HungerLabel:           "Hunger",
		ProfessionLabel:       "Profession",
		HomeLabel:             "Workplace",
		BuildingLabel:         "Building",
		InputLabel:            "Input",
		OutputLabel:           "Output",
		RoadLabel:             "Road",
		Connected:             "connected",
		Disconnected:          "not connected",
		NoCargo:               "no cargo",
		NoRoute:               "no route",
		StateIdle:             "idle",
		StateWorking:          "working",
		StateWalking:          "walking",
		StateDelivering:       "delivering",
		StateEating:           "eating",
		StateStarving:         "starving",
		UnitSerf:              "Serf",
		UnitFarmer:            "Farmer",
		UnitBaker:             "Baker",
		HireSerf:              "Hire serf [H]",
		Deleted:               "Deleted.",
		CannotDeleteWarehouse: "The warehouse cannot be deleted",

		Help:                  "Arrows: pan | 1-5: select | Click: place/select | H: hire serf | Delete: remove | S/L: save/load | Esc: quit",
		CantBuildHere:         "Can't build there",
		SaveFailedPrefix:      "Save failed: ",
		LoadFailedPrefix:      "Load failed: ",
		Saved:                 "Saved.",
		Loaded:                "Loaded.",
		LoadFailedNoWarehouse: "Load failed: save has no warehouse",
	})
}
