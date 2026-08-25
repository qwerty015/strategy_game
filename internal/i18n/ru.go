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
			resource.Wheat:   "Пшеница",
			resource.Flour:   "Мука",
			resource.Bread:   "Хлеб",
			resource.Fish:    "Рыба",
			resource.Wine:    "Вино",
			resource.Sausage: "Колбаса",
		},
		BuildingName: map[building.Kind]string{
			building.Farm:      "Ферма",
			building.Mill:      "Мельница",
			building.Bakery:    "Пекарня",
			building.Warehouse: "Склад",
			building.Road:      "Дорога",
			building.Tavern:    "Харчевня",
			building.Tree:      "Дерево",
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

		StateLabel:            "Состояние",
		CargoLabel:            "Груз",
		RouteLabel:            "Маршрут",
		HungerLabel:           "Сытость",
		ProfessionLabel:       "Профессия",
		HomeLabel:             "Рабочее место",
		BuildingLabel:         "Здание",
		InputLabel:            "Вход",
		OutputLabel:           "Выход",
		ContentsLabel:         "Внутри",
		UnlimitedLabel:        "без лимита",
		GrowthLabel:           "Рост",
		IndestructibleLabel:   "Неубираемое",
		RoadLabel:             "Дорога",
		Connected:             "подключена",
		Disconnected:          "не подключена",
		NoCargo:               "нет груза",
		NoRoute:               "нет маршрута",
		StateIdle:             "стоит",
		StateWorking:          "работает",
		StateWalking:          "идёт",
		StateDelivering:       "доставляет",
		StateEating:           "ест",
		StateStarving:         "голодает",
		UnitSerf:              "Слуга",
		UnitFarmer:            "Фермер",
		UnitBaker:             "Пекарь",
		HireSerf:              "Нанять слугу [H]",
		Deleted:               "Удалено.",
		CannotDeleteWarehouse: "Склад нельзя удалить",
		CannotDeleteTree:      "Дерево пока нельзя убрать",

		Help:                  "Стрелки/СКМ: камера | Колесо +/-: масштаб | 1-6: выбор | Клик: построить/выбрать | H: нанять слугу | Delete: удалить | S/L: сохранение/загрузка | Esc: выход",
		CantBuildHere:         "Здесь нельзя строить",
		SaveFailedPrefix:      "Не удалось сохранить: ",
		LoadFailedPrefix:      "Не удалось загрузить: ",
		Saved:                 "Сохранено.",
		Loaded:                "Загружено.",
		LoadFailedNoWarehouse: "Не удалось загрузить: в сохранении нет склада",
	})
}
