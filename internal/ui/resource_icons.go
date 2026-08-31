package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/i18n"
	"strategy_game/internal/resource"
)

// resourceIconSize is deliberately small: warehouse rows stay readable even
// with every current and future material visible at once.
const resourceIconSize = 14

var (
	resourceIconBack   = color.RGBA{R: 42, G: 35, B: 31, A: 255}
	resourceIconBorder = color.RGBA{R: 118, G: 92, B: 57, A: 255}
	iconWheat          = color.RGBA{R: 226, G: 186, B: 54, A: 255}
	iconFlour          = color.RGBA{R: 231, G: 218, B: 180, A: 255}
	iconBread          = color.RGBA{R: 185, G: 105, B: 48, A: 255}
	iconFish           = color.RGBA{R: 83, G: 169, B: 197, A: 255}
	iconWine           = color.RGBA{R: 142, G: 66, B: 116, A: 255}
	iconMeat           = color.RGBA{R: 194, G: 78, B: 63, A: 255}
	iconWood           = color.RGBA{R: 135, G: 79, B: 38, A: 255}
	iconStone          = color.RGBA{R: 142, G: 145, B: 151, A: 255}
	iconGold           = color.RGBA{R: 244, G: 198, B: 44, A: 255}
	iconCoal           = color.RGBA{R: 61, G: 67, B: 72, A: 255}
	iconIron           = color.RGBA{R: 172, G: 185, B: 192, A: 255}
)

// resourceTooltip is collected while panels render and drawn once on top of
// all side UI at the end of the frame. That avoids a tooltip being hidden by
// a later panel or minimap draw call.
var resourceTooltip struct {
	active bool
	kind   resource.Type
	x, y   int
}

// BeginResourceTooltips clears the previous frame's hover target. Every UI
// frame must call it before drawing a resource icon.
func BeginResourceTooltips() {
	resourceTooltip.active = false
}

// DrawResourceTooltip renders the localized name of the resource under the
// cursor. Values stay next to their icons in the panel; the tooltip's purpose
// is to identify the compact visual glyph without duplicating every label.
func DrawResourceTooltip(screen *ebiten.Image, layout Layout) {
	if !resourceTooltip.active {
		return
	}
	label := i18n.T().ResourceName[resourceTooltip.kind]
	width := 28 + len([]rune(label))*8
	if width < 92 {
		width = 92
	}
	const height = 22
	x, y := resourceTooltip.x+14, resourceTooltip.y+14
	if x+width > layout.Width-4 {
		x = layout.Width - width - 4
	}
	if y+height > layout.Height-4 {
		y = resourceTooltip.y - height - 8
	}
	if x < 4 {
		x = 4
	}
	if y < 4 {
		y = 4
	}
	fillIconRect(screen, x, y, width, height, panelColor)
	fillIconRect(screen, x, y, width, 1, panelEdgeColor)
	fillIconRect(screen, x, y+height-1, width, 1, panelEdgeColor)
	fillIconRect(screen, x, y, 1, height, panelEdgeColor)
	fillIconRect(screen, x+width-1, y, 1, height, panelEdgeColor)
	drawResourceIcon(screen, resourceTooltip.kind, x+4, y+4)
	DrawInspectorText(screen, label, float64(x+resourceIconSize+10), float64(y+4))
}

// drawResourceIcon draws an authored tiny resource PNG when one is available,
// falling back to the procedural glyph below for an incomplete asset bundle.
func drawResourceIcon(screen *ebiten.Image, kind resource.Type, x, y int) {
	mx, my := ebiten.CursorPosition()
	if mx >= x && mx < x+resourceIconSize && my >= y && my < y+resourceIconSize {
		resourceTooltip.active = true
		resourceTooltip.kind = kind
		resourceTooltip.x, resourceTooltip.y = mx, my
	}
	fillIconRect(screen, x, y, resourceIconSize, resourceIconSize, resourceIconBack)
	fillIconRect(screen, x, y, resourceIconSize, 1, resourceIconBorder)
	fillIconRect(screen, x, y+resourceIconSize-1, resourceIconSize, 1, resourceIconBorder)
	fillIconRect(screen, x, y, 1, resourceIconSize, resourceIconBorder)
	fillIconRect(screen, x+resourceIconSize-1, y, 1, resourceIconSize, resourceIconBorder)

	if image := assets.ResourceIcon(kind); image != nil {
		bounds := image.Bounds()
		scale := float64(resourceIconSize-2) / float64(bounds.Dx())
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(x+1), float64(y+1))
		op.Blend = ebiten.BlendSourceOver
		screen.DrawImage(image, op)
		return
	}

	switch kind {
	case resource.Wheat:
		fillIconRect(screen, x+6, y+2, 2, 10, iconWheat)
		fillIconRect(screen, x+3, y+4, 3, 1, iconWheat)
		fillIconRect(screen, x+8, y+6, 3, 1, iconWheat)
		fillIconRect(screen, x+3, y+8, 3, 1, iconWheat)
	case resource.Flour:
		fillIconRect(screen, x+4, y+3, 6, 8, iconFlour)
		fillIconRect(screen, x+5, y+2, 4, 1, iconFlour)
		fillIconRect(screen, x+5, y+6, 4, 1, iconWood)
	case resource.Bread:
		fillIconRect(screen, x+3, y+5, 8, 5, iconBread)
		fillIconRect(screen, x+4, y+4, 6, 1, iconBread)
		fillIconRect(screen, x+5, y+6, 1, 2, iconFlour)
		fillIconRect(screen, x+8, y+6, 1, 2, iconFlour)
	case resource.Fish:
		fillIconRect(screen, x+3, y+6, 8, 3, iconFish)
		fillIconRect(screen, x+2, y+7, 1, 1, iconFish)
		fillIconRect(screen, x+11, y+5, 1, 5, iconFish)
		fillIconRect(screen, x+12, y+6, 1, 3, iconFish)
	case resource.Wine:
		fillIconRect(screen, x+6, y+2, 2, 2, iconWine)
		fillIconRect(screen, x+5, y+4, 4, 7, iconWine)
		fillIconRect(screen, x+4, y+6, 1, 4, iconWine)
		fillIconRect(screen, x+9, y+6, 1, 4, iconWine)
	case resource.Sausage:
		fillIconRect(screen, x+3, y+6, 8, 3, iconMeat)
		fillIconRect(screen, x+2, y+7, 1, 1, iconFlour)
		fillIconRect(screen, x+11, y+7, 1, 1, iconFlour)
	case resource.Carcass:
		fillIconRect(screen, x+4, y+4, 6, 6, iconMeat)
		fillIconRect(screen, x+3, y+6, 1, 3, iconMeat)
		fillIconRect(screen, x+10, y+5, 1, 4, iconFlour)
	case resource.Log:
		fillIconRect(screen, x+2, y+6, 10, 4, iconWood)
		fillIconRect(screen, x+3, y+7, 1, 2, iconFlour)
		fillIconRect(screen, x+10, y+7, 1, 2, iconFlour)
	case resource.Plank:
		fillIconRect(screen, x+3, y+4, 8, 3, iconWood)
		fillIconRect(screen, x+3, y+8, 8, 2, iconWood)
		fillIconRect(screen, x+5, y+4, 1, 3, iconFlour)
	case resource.StoneBlock:
		fillIconRect(screen, x+3, y+4, 8, 7, iconStone)
		fillIconRect(screen, x+4, y+5, 6, 1, iconIron)
		fillIconRect(screen, x+6, y+7, 1, 4, iconCoal)
	case resource.Gold:
		fillIconRect(screen, x+4, y+4, 6, 7, iconGold)
		fillIconRect(screen, x+5, y+3, 4, 1, iconGold)
		fillIconRect(screen, x+5, y+6, 4, 1, iconFlour)
	case resource.Coal:
		fillIconRect(screen, x+4, y+4, 6, 7, iconCoal)
		fillIconRect(screen, x+3, y+6, 1, 3, iconCoal)
		fillIconRect(screen, x+10, y+5, 1, 4, iconCoal)
	case resource.GoldOre:
		drawOreIcon(screen, x, y, iconGold)
	case resource.IronOre:
		drawOreIcon(screen, x, y, iconIron)
	case resource.Iron:
		fillIconRect(screen, x+4, y+4, 6, 7, iconIron)
		fillIconRect(screen, x+5, y+3, 4, 1, iconIron)
		fillIconRect(screen, x+5, y+6, 4, 1, iconFlour)
	}
}

func drawOreIcon(screen *ebiten.Image, x, y int, ore color.Color) {
	fillIconRect(screen, x+3, y+5, 8, 6, iconStone)
	fillIconRect(screen, x+4, y+4, 6, 1, iconStone)
	fillIconRect(screen, x+5, y+6, 2, 2, ore)
	fillIconRect(screen, x+8, y+8, 2, 2, ore)
}

func fillIconRect(screen *ebiten.Image, x, y, width, height int, c color.Color) {
	vector.FillRect(screen, float32(x), float32(y), float32(width), float32(height), c, false)
}

// drawResourceRow draws a resource icon followed by arbitrary inspector
// text at the usual indent -- the same layout drawWarehouseResourceRow
// already used just for the Warehouse, generalized to any caller-supplied
// text so every other building's "name: current/max" buffer line can use
// it too, not just the Warehouse's plain "name: amount".
func drawResourceRow(screen *ebiten.Image, x, y int, kind resource.Type, text string) {
	drawResourceIcon(screen, kind, x, y+1)
	DrawInspectorText(screen, text, float64(x+resourceIconSize+6), float64(y))
}

func drawWarehouseResourceRow(screen *ebiten.Image, x, y int, kind resource.Type, amount int) {
	drawResourceRow(screen, x, y, kind, fmt.Sprintf("%s: %d", i18n.T().ResourceName[kind], amount))
}
