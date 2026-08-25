package i18n

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func init() {
	register(RU, Catalog{
		WindowTitle: "Градостроитель",

		Population: "Население",
		ResourceName: map[resource.Type]string{
			resource.Wheat: "Пшеница",
			resource.Flour: "Мука",
			resource.Bread: "Хлеб",
		},
		BuildingName: map[building.Kind]string{
			building.Farm:      "Ферма",
			building.Mill:      "Мельница",
			building.Bakery:    "Пекарня",
			building.Warehouse: "Склад",
			building.Road:      "Дорога",
			building.Tavern:    "Харчевня",
		},

		BuildMenuTitle: "Строительство",
		InspectorTitle: "Сведения",
		InspectorHint:  "Выберите здание или юнита",
		SpeedTitle:     "Скорость",
		SpeedPaused:    "Пауза",
		SpeedHalf:      "0,5x",
		SpeedNormal:    "1x",
		SpeedDouble:    "2x",
		SpeedQuadruple: "4x",

		StateLabel:      "Состояние",
		CargoLabel:      "Груз",
		RouteLabel:      "Маршрут",
		HungerLabel:     "Сытость",
		ProfessionLabel: "Профессия",
		HomeLabel:       "Рабочее место",
		BuildingLabel:   "Здание",
		InputLabel:      "Вход",
		OutputLabel:     "Выход",
		NoCargo:         "нет груза",
		NoRoute:         "нет маршрута",
		StateIdle:       "стоит",
		StateWorking:    "работает",
		StateWalking:    "идёт",
		StateDelivering: "доставляет",
		StateEating:     "ест",
		StateStarving:   "голодает",
		UnitSerf:        "Слуга",
		UnitFarmer:      "Фермер",
		UnitBaker:       "Пекарь",

		Help:                  "Стрелки: камера | 1-5: выбор постройки | Клик: построить | S: сохранить | L: загрузить | Esc: выход",
		CantBuildHere:         "Здесь нельзя строить",
		SaveFailedPrefix:      "Не удалось сохранить: ",
		LoadFailedPrefix:      "Не удалось загрузить: ",
		Saved:                 "Сохранено.",
		Loaded:                "Загружено.",
		LoadFailedNoWarehouse: "Не удалось загрузить: в сохранении нет склада",
	})
}
