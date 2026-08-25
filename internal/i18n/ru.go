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
		},

		Help:                  "Стрелки: камера | 1-4: выбор постройки | Клик: построить | S: сохранить | L: загрузить | Esc: выход",
		CantBuildHere:         "Здесь нельзя строить",
		SaveFailedPrefix:      "Не удалось сохранить: ",
		LoadFailedPrefix:      "Не удалось загрузить: ",
		Saved:                 "Сохранено.",
		Loaded:                "Загружено.",
		LoadFailedNoWarehouse: "Не удалось загрузить: в сохранении нет склада",
	})
}
