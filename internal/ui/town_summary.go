package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/resource"
)

const (
	townSummaryIconSize = 16
	playFramesPerSecond = 60
)

type townSummaryIcon int

const (
	townSummaryPopulation townSummaryIcon = iota
	townSummaryDeaths
	townSummaryBuildingsRemoved
	townSummaryUnitsDismissed
	townSummaryBuildings
	townSummaryPlayTime
)

// drawTownSummary fills the otherwise empty inspector with persistent town
// information. The stockpile is shared by every operational warehouse, so its
// values are exactly the total resources the player can spend or deliver.
func drawTownSummary(screen *ebiten.Image, panelX, panelWidth int, stock *resource.Stockpile, pop *economy.Population, buildings, playedFrames int) {
	x := panelX + 18
	t := i18n.T()
	DrawInspectorText(screen, t.TownSummaryLabel, float64(x), 62)

	population, deaths, buildingsRemoved, unitsDismissed := 0, 0, 0, 0
	if pop != nil {
		population = pop.Count
		deaths = pop.Deaths
		buildingsRemoved = pop.BuildingsRemoved
		unitsDismissed = pop.UnitsDismissed
	}
	stats := []struct {
		icon  townSummaryIcon
		label string
	}{
		{townSummaryPopulation, fmt.Sprintf("%s: %d", t.Population, population)},
		{townSummaryDeaths, fmt.Sprintf("%s: %d", t.DeathsLabel, deaths)},
		{townSummaryBuildingsRemoved, fmt.Sprintf("%s: %d", t.BuildingsRemovedLabel, buildingsRemoved)},
		{townSummaryUnitsDismissed, fmt.Sprintf("%s: %d", t.UnitsDismissedLabel, unitsDismissed)},
		{townSummaryBuildings, fmt.Sprintf("%s: %d", t.BuildingsLabel, buildings)},
		{townSummaryPlayTime, fmt.Sprintf("%s: %s", t.PlayTimeLabel, formatPlayedFrames(playedFrames))},
	}
	for i, stat := range stats {
		y := 82 + i*20
		drawTownSummaryIcon(screen, stat.icon, x, y)
		DrawInspectorText(screen, stat.label, float64(x+townSummaryIconSize+7), float64(y))
	}

	dividerY := 208
	fillIconRect(screen, x, dividerY, panelWidth-36, 2, panelEdgeColor)
	DrawInspectorText(screen, t.ResourcesInWarehousesLabel, float64(x), float64(dividerY+12))

	const columns = 3
	columnWidth := (panelWidth - 36) / columns
	for i, kind := range resource.AllTypes() {
		column, row := i%columns, i/columns
		cellX := x + column*columnWidth
		cellY := dividerY + 32 + row*20
		amount := 0
		if stock != nil {
			amount = stock.Amount(kind)
		}
		drawResourceIcon(screen, kind, cellX, cellY)
		DrawInspectorText(screen, fmt.Sprintf("%d", amount), float64(cellX+resourceIconSize+5), float64(cellY))
	}
}

// formatPlayedFrames returns a language-neutral clock value (H:MM:SS). The
// counter advances only during active gameplay, not while a menu is open.
func formatPlayedFrames(frames int) string {
	if frames < 0 {
		frames = 0
	}
	seconds := frames / playFramesPerSecond
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	seconds %= 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

// drawTownSummaryIcon draws compact visual labels for every town counter.
// Resources use authored resource PNGs; these five summary glyphs are simple
// UI pictograms because they describe game statistics rather than world items.
func drawTownSummaryIcon(screen *ebiten.Image, kind townSummaryIcon, x, y int) {
	switch kind {
	case townSummaryPopulation, townSummaryUnitsDismissed:
		drawHireIcon(screen, HireSerf, x, y, townSummaryIconSize)
		return
	case townSummaryBuildings:
		drawBuildingIcon(screen, building.Warehouse, x, y, townSummaryIconSize)
		return
	}

	fillIconRect(screen, x, y, townSummaryIconSize, townSummaryIconSize, resourceIconBack)
	fillIconRect(screen, x, y, townSummaryIconSize, 1, resourceIconBorder)
	fillIconRect(screen, x, y+townSummaryIconSize-1, townSummaryIconSize, 1, resourceIconBorder)
	fillIconRect(screen, x, y, 1, townSummaryIconSize, resourceIconBorder)
	fillIconRect(screen, x+townSummaryIconSize-1, y, 1, townSummaryIconSize, resourceIconBorder)

	switch kind {
	case townSummaryDeaths:
		for i := 0; i < 8; i++ {
			fillIconRect(screen, x+4+i, y+4+i, 2, 2, color.RGBA{R: 183, G: 65, B: 55, A: 255})
			fillIconRect(screen, x+10-i, y+4+i, 2, 2, color.RGBA{R: 183, G: 65, B: 55, A: 255})
		}
	case townSummaryBuildingsRemoved:
		fillIconRect(screen, x+4, y+5, 8, 7, color.RGBA{R: 137, G: 117, B: 93, A: 255})
		fillIconRect(screen, x+5, y+4, 6, 2, color.RGBA{R: 184, G: 151, B: 90, A: 255})
		fillIconRect(screen, x+3, y+11, 10, 2, color.RGBA{R: 183, G: 65, B: 55, A: 255})
	case townSummaryPlayTime:
		fillIconRect(screen, x+4, y+3, 8, 10, color.RGBA{R: 218, G: 190, B: 119, A: 255})
		fillIconRect(screen, x+5, y+4, 6, 8, color.RGBA{R: 74, G: 63, B: 54, A: 255})
		fillIconRect(screen, x+7, y+5, 1, 4, color.RGBA{R: 228, G: 214, B: 180, A: 255})
		fillIconRect(screen, x+7, y+8, 3, 1, color.RGBA{R: 228, G: 214, B: 180, A: 255})
	}
}
