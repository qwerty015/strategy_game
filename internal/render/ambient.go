package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/world"
	"strategy_game/internal/worldclock"
)

const (
	hareRunDuration = 300
	hareRunTiles    = 7
	foxRunDuration  = 480
	foxRunTiles     = 9
)

// DrawAmbientGroundLife renders wildlife that belongs to the landscape. Hares
// sit below buildings and roads, so they naturally disappear behind solid
// objects instead of looking as if they run through them.
func DrawAmbientGroundLife(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	drawAmbientFox(screen, g, cam)
	drawAmbientHares(screen, g, cam)
}

// drawAmbientFox adds one occasional, visible fox run on an open grassy lane.
// Unlike a map-wide simulation animal, its lane is picked only within the
// camera's current view and disappears naturally below roads and buildings.
func drawAmbientFox(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	if g.Width < foxRunTiles+1 || g.Height == 0 {
		return
	}
	visible := cam.VisibleTileBounds(1)
	cycle := animFrame / foxRunDuration
	startX, y, direction, ok := findVisibleFoxRun(g, visible, ambientHash(uint32(cycle)*3266489917))
	if !ok {
		return
	}
	progress := float64(animFrame%foxRunDuration) / foxRunDuration
	x := float64(startX) + float64(direction)*progress*foxRunTiles
	tileX := int(math.Floor(x))
	if !visible.Intersects(tileX, y, 1) || !g.InBounds(tileX, y) || g.At(tileX, y).Terrain != world.Grass {
		return
	}
	sx, sy := cam.TileToScreen(tileX, y)
	sx += (x - math.Floor(x)) * cam.TilePixels()
	frame := (animFrame / 8) % len(assets.FoxFrames)
	drawStandingFacingTintedAtScale(screen, assets.FoxFrames[frame], sx, sy, 0.58, cam.TilePixels(), color.White, direction < 0)
}

func findVisibleFoxRun(g *world.Grid, visible TileBounds, seed uint32) (startX, y, direction int, ok bool) {
	minLeft := visible.MinX - foxRunTiles
	if minLeft < 0 {
		minLeft = 0
	}
	maxLeft := visible.MaxX
	if limit := g.Width - foxRunTiles - 1; maxLeft > limit {
		maxLeft = limit
	}
	minY := visible.MinY
	if minY < 0 {
		minY = 0
	}
	maxY := visible.MaxY
	if maxY >= g.Height {
		maxY = g.Height - 1
	}
	if maxLeft < minLeft || maxY < minY {
		return 0, 0, 0, false
	}
	for attempt := 0; attempt < 14; attempt++ {
		hash := ambientHash(seed + uint32(attempt)*668265263)
		left := minLeft + int((hash>>7)%uint32(maxLeft-minLeft+1))
		y = minY + int((hash>>17)%uint32(maxY-minY+1))
		direction = 1
		if hash&1 != 0 {
			direction = -1
			startX = left + foxRunTiles
		} else {
			startX = left
		}
		clear := true
		for step := 0; step <= foxRunTiles; step++ {
			x := startX + direction*step
			if !g.InBounds(x, y) || g.At(x, y).Terrain != world.Grass {
				clear = false
				break
			}
		}
		if clear {
			return startX, y, direction, true
		}
	}
	return 0, 0, 0, false
}

// DrawAmbientSkyLife renders a small visible flock after world objects. Birds
// are deliberately above the map layer so foliage and tall buildings cannot
// hide every one of them; UI panels are still drawn later and remain on top.
func DrawAmbientSkyLife(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	drawAmbientButterflies(screen, g, cam)
	drawAmbientBirds(screen, g, cam)
	drawAmbientFireflies(screen, g, cam)
}

// drawAmbientButterflies gives sunny fields a little movement without adding
// an NPC or a map-wide update. Their short routes are recomputed from the
// current animation cycle and visible tile range, exactly like birds/hares.
func drawAmbientButterflies(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	if currentDayState().Phase != worldclock.Day || g.Width == 0 || g.Height == 0 {
		return
	}
	visible := cam.VisibleTileBounds(0)
	viewWidth := visible.MaxX - visible.MinX + 1
	viewHeight := visible.MaxY - visible.MinY + 1
	if viewWidth < 1 || viewHeight < 1 {
		return
	}
	cycle := animFrame / 260
	tilePixels := cam.TilePixels()
	for index := 0; index < 4; index++ {
		seed := ambientHash(uint32(cycle)*2654435761 + uint32(index)*2246822519)
		tileX := visible.MinX + int(seed%uint32(viewWidth))
		tileY := visible.MinY + int((seed>>12)%uint32(viewHeight))
		if !g.InBounds(tileX, tileY) || g.At(tileX, tileY).Terrain != world.Grass {
			continue
		}
		sx, sy := cam.TileToScreen(tileX, tileY)
		flutter := float64((animFrame/9+index*3)%7) / 7
		sx += tilePixels * (0.16 + 0.48*flutter)
		sy += tilePixels * (0.28 + 0.16*float64((seed>>24)&3)/3)
		frame := (animFrame/6 + index) % len(assets.ButterflyFrames)
		drawStandingFacingTintedAtScale(screen, assets.ButterflyFrames[frame], sx, sy, 0.29, tilePixels, color.White, seed&1 != 0)
	}
}

// fireflyCount was 10 before the user asked for more of them at night
// ("и светлячков ночью сделай больше") now that they're actually reachable
// (see the Phase gate below -- Night now recurs every in-game day, unlike
// the old evening-only window that in practice was almost never seen).
const fireflyCount = 26

// drawAmbientFireflies appears only at night. It uses a fixed candidate
// count and no persistent state.
func drawAmbientFireflies(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	if currentDayState().Phase != worldclock.Night || g.Width == 0 || g.Height == 0 {
		return
	}
	visible := cam.VisibleTileBounds(0)
	viewWidth := visible.MaxX - visible.MinX + 1
	viewHeight := visible.MaxY - visible.MinY + 1
	if viewWidth < 1 || viewHeight < 1 {
		return
	}
	tilePixels := cam.TilePixels()
	cycle := animFrame / 180
	for index := 0; index < fireflyCount; index++ {
		seed := ambientHash(uint32(cycle)*3266489917 + uint32(index)*668265263)
		tileX := visible.MinX + int(seed%uint32(viewWidth))
		tileY := visible.MinY + int((seed>>11)%uint32(viewHeight))
		if !g.InBounds(tileX, tileY) || g.At(tileX, tileY).Terrain != world.Grass {
			continue
		}
		sx, sy := cam.TileToScreen(tileX, tileY)
		glow := color.RGBA{R: 246, G: 229, B: 109, A: 205}
		x := float32(sx + tilePixels*(0.18+0.62*float64((seed>>20)&0xff)/255))
		y := float32(sy + tilePixels*(0.20+0.52*float64((seed>>8)&0xff)/255))
		radius := float32(maxPixel(tilePixels * 0.042))
		vector.FillCircle(screen, x, y, radius*1.8, color.RGBA{R: 226, G: 211, B: 85, A: 38}, false)
		vector.FillCircle(screen, x, y, radius, glow, false)
	}
}

func drawAmbientHares(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	if g.Width < hareRunTiles+1 || g.Height == 0 {
		return
	}
	cycle := animFrame / hareRunDuration
	progress := float64(animFrame%hareRunDuration) / hareRunDuration
	visible := cam.VisibleTileBounds(1)
	tilePixels := cam.TilePixels()

	// Two candidates at a time are enough to make the land feel inhabited
	// without turning wildlife into a repeated visual effect on every screen.
	for index := 0; index < 2; index++ {
		seed := ambientHash(uint32(cycle)*2654435761 + uint32(index)*668265263)
		startX, y, direction, ok := findHareRun(g, seed)
		if !ok {
			continue
		}
		x := float64(startX) + float64(direction)*progress*hareRunTiles
		tileX := int(math.Floor(x))
		if !visible.Intersects(tileX, y, 1) || !g.InBounds(tileX, y) || g.At(tileX, y).Terrain != world.Grass {
			continue
		}
		sx, sy := cam.TileToScreen(tileX, y)
		sx += (x - math.Floor(x)) * tilePixels
		frame := (animFrame/7 + index) % len(assets.HareFrames)
		drawStandingFacingTintedAtScale(screen, assets.HareFrames[frame], sx, sy, 0.48, tilePixels, color.White, direction < 0)
	}
}

// findHareRun chooses an entirely grass, horizontal seven-tile lane. A hare
// therefore never visibly crosses water, a road or a field, and no state has
// to be kept while it is off-screen.
func findHareRun(g *world.Grid, seed uint32) (startX, y, direction int, ok bool) {
	if g.Width < hareRunTiles+1 || g.Height == 0 {
		return 0, 0, 0, false
	}
	for attempt := 0; attempt < 18; attempt++ {
		hash := ambientHash(seed + uint32(attempt)*2246822519)
		direction = 1
		if hash&1 != 0 {
			direction = -1
		}
		y = int((hash >> 8) % uint32(g.Height))
		left := int((hash >> 16) % uint32(g.Width-hareRunTiles))
		if direction < 0 {
			startX = left + hareRunTiles
		} else {
			startX = left
		}
		clear := true
		for step := 0; step <= hareRunTiles; step++ {
			x := startX + direction*step
			if !g.InBounds(x, y) || g.At(x, y).Terrain != world.Grass {
				clear = false
				break
			}
		}
		if clear {
			return startX, y, direction, true
		}
	}
	return 0, 0, 0, false
}

// drawAmbientBirds schedules two small flocks inside the current viewport,
// not across the entire world width. The previous map-wide route made birds
// absent for almost the whole cycle on a large map.
func drawAmbientBirds(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	if g.Width == 0 || g.Height == 0 {
		return
	}
	visible := cam.VisibleTileBounds(0)
	if visible.MaxX < visible.MinX || visible.MaxY < visible.MinY {
		return
	}
	tilePixels := cam.TilePixels()
	viewWidth := visible.MaxX - visible.MinX + 1
	viewHeight := visible.MaxY - visible.MinY + 1
	if viewWidth < 1 {
		viewWidth = 1
	}
	if viewHeight < 1 {
		viewHeight = 1
	}
	cycle := animFrame / 420
	formation := [...]struct {
		x, y, scale float64
	}{
		{0, 0, 1},
		{-0.34, 0.12, 0.76},
		{0.30, 0.16, 0.70},
	}
	for flock := 0; flock < 2; flock++ {
		seed := ambientHash(uint32(cycle)*3266489917 + uint32(flock)*668265263)
		progress := math.Mod(float64(animFrame%420)/420+float64(flock)*0.51, 1)
		x := float64(visible.MinX-1) + progress*float64(viewWidth+2)
		y := visible.MinY + int((seed>>10)%uint32(viewHeight))
		if y < 0 || y >= g.Height {
			continue
		}
		baseX, baseY := cam.TileToScreen(int(math.Floor(x)), y)
		baseX += (x-math.Floor(x))*tilePixels + tilePixels*0.5
		baseY += tilePixels * (0.18 + 0.12*float64((seed>>6)&1))
		flutter := float64((animFrame/8+flock)%3-1) * tilePixels * 0.025
		for _, bird := range formation {
			drawBirdSilhouette(screen, baseX+bird.x*tilePixels, baseY+(bird.y*tilePixels)+flutter, tilePixels*bird.scale*1.28)
		}
	}
}

func drawBirdSilhouette(screen *ebiten.Image, x, y, size float64) {
	wing := float32(size * 0.30)
	stroke := float32(maxPixel(size * 0.058))
	bird := color.RGBA{R: 39, G: 35, B: 31, A: 238}
	cx, cy := float32(x), float32(y)
	vector.StrokeLine(screen, cx-wing, cy, cx, cy-wing*0.48, stroke, bird, true)
	vector.StrokeLine(screen, cx, cy-wing*0.48, cx+wing, cy, stroke, bird, true)
}

// ambientHash is a tiny integer mixer for visual-only scheduling. It avoids
// allocating random generators and is stable for the same cycle and tile.
func ambientHash(value uint32) uint32 {
	value ^= value >> 16
	value *= 0x7feb352d
	value ^= value >> 15
	value *= 0x846ca68b
	value ^= value >> 16
	return value
}
