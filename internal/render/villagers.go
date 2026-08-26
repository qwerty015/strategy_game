package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/villagers"
)

// DrawVillagers renders every Farmer/Baker/Winemaker at its current tile. A
// field worker walks a small loop over the eight crop cells while working; a baker remains
// represented by the building marker until it leaves for the Tavern. The
// farmer's bobbing pose and tiny tool cue are intentionally lightweight
// pseudo-animation, but make sowing/harvesting readable at game scale.
func DrawVillagers(screen *ebiten.Image, vills []*villagers.Villager, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, v := range vills {
		// A working baker is represented by the building marker. A working
		// farmer is the exception: its visible field route is part of the
		// feedback for farm work.
		if v.Working() && !v.VisibleOnMap() {
			continue
		}
		frames := assets.Farmer
		switch v.Profession {
		case villagers.Baker:
			frames = assets.Baker
		case villagers.Winemaker:
			frames = assets.Winemaker
		}

		sx, sy := cam.TileToScreen(v.X, v.Y)

		frame := 0
		bob := 0.0
		tint := color.Color(color.White)
		switch {
		case v.Starving:
			tint = color.RGBA{R: 255, G: 90, B: 90, A: 255}
		case v.Working():
			frame = (animFrame / 10) % len(frames)
			bob = unitBob()
		case !v.Working():
			frame = (animFrame / 10) % len(frames)
			bob = unitBob()
		}
		drawStandingTintedAtScale(screen, frames[frame], sx, sy+bob*tilePixels/TileSize, serfHeight, tilePixels, tint)

		if v.Working() && (v.Profession == villagers.Farmer || v.Profession == villagers.Winemaker) {
			drawFarmWorkCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
		}
	}
}

func drawFarmWorkCue(screen *ebiten.Image, sx, sy, tilePixels float64) {
	phase := (animFrame / 12) % 3
	if phase == 0 {
		return
	}
	scale := tilePixels / TileSize
	if phase == 1 {
		// Ochre pixels read as a hoe/harvest stroke beside the farmer.
		vector.FillRect(screen, float32(sx+17*scale), float32(sy+12*scale), float32(maxPixel(scale)), float32(5*scale), color.RGBA{R: 196, G: 149, B: 52, A: 230}, false)
		return
	}
	// Green-gold flecks suggest newly sown or cut stalks.
	vector.FillRect(screen, float32(sx+19*scale), float32(sy+8*scale), float32(maxPixel(scale)), float32(maxPixel(scale)), color.RGBA{R: 232, G: 201, B: 74, A: 235}, false)
	vector.FillRect(screen, float32(sx+22*scale), float32(sy+11*scale), float32(maxPixel(scale)), float32(maxPixel(scale)), color.RGBA{R: 102, G: 156, B: 54, A: 230}, false)
}

func maxPixel(scale float64) float32 {
	if scale < 1 {
		return 1
	}
	return float32(scale)
}
