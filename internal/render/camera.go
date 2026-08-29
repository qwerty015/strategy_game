// Package render turns game state (world.Grid, buildings) into pixels on
// an ebiten screen. It's the only place besides cmd/game and ui that is
// allowed to depend on ebiten.
package render

import "math"

const TileSize = 24 // pixels per tile side, at default zoom

const (
	minZoom = 0.60
	maxZoom = 2.50
)

// Camera tracks which part of the world (in pixels) is visible at the
// top-left of the screen, and converts between world and screen space.
type Camera struct {
	X, Y  float64 // world-space pixel offset of the viewport's top-left
	Scale float64 // zoom multiplier; 1.0 is the default tile size

	viewportX, viewportY          int
	viewportWidth, viewportHeight int
}

// TileBounds is the inclusive world-tile rectangle currently relevant to a
// render pass. It lets each renderer discard off-screen entities before it
// allocates draw options or submits a GPU draw call.
type TileBounds struct {
	MinX, MinY int
	MaxX, MaxY int
}

// Intersects reports whether a square building footprint overlaps these
// visible tiles. Footprints are always at least one tile wide in the building
// registry; a defensive zero value remains one tile for callers in UI code.
func (b TileBounds) Intersects(x, y, footprint int) bool {
	if footprint < 1 {
		footprint = 1
	}
	return x <= b.MaxX && y <= b.MaxY && x+footprint-1 >= b.MinX && y+footprint-1 >= b.MinY
}

// NewCamera creates a camera positioned at the world origin.
func NewCamera() *Camera {
	return &Camera{Scale: 1}
}

// SetViewport tells the camera where the map is drawn on the screen. UI
// panels can then occupy the edges without desynchronizing map placement.
func (c *Camera) SetViewport(x, y, width, height int) {
	c.viewportX, c.viewportY = x, y
	c.viewportWidth, c.viewportHeight = width, height
}

// Viewport returns the on-screen rectangle owned by the map. Ambient overlays
// use it instead of the whole window, so weather can never tint UI panels.
func (c *Camera) Viewport() (x, y, width, height int) {
	return c.viewportX, c.viewportY, c.viewportWidth, c.viewportHeight
}

// TilePixels is the current on-screen size of one world tile.
func (c *Camera) TilePixels() float64 {
	scale := c.Scale
	if scale <= 0 {
		scale = 1
	}
	return TileSize * scale
}

// ZoomAt changes zoom while keeping the world point under the cursor fixed.
// That makes mouse-wheel zoom feel attached to the map instead of pulling
// the camera toward an unrelated corner.
func (c *Camera) ZoomAt(delta float64, screenX, screenY int, gridWidth, gridHeight int) {
	oldScale := c.Scale
	if oldScale <= 0 {
		oldScale = 1
	}
	newScale := oldScale
	if delta > 0 {
		newScale *= 1.15
	} else if delta < 0 {
		newScale /= 1.15
	}
	newScale = clamp(newScale, minZoom, maxZoom)
	if newScale == oldScale {
		return
	}

	worldX := c.X + float64(screenX-c.viewportX)/oldScale
	worldY := c.Y + float64(screenY-c.viewportY)/oldScale
	c.Scale = newScale
	c.X = worldX - float64(screenX-c.viewportX)/newScale
	c.Y = worldY - float64(screenY-c.viewportY)/newScale
	c.clamp(gridWidth, gridHeight)
}

// SetZoom changes zoom to an absolute scale (clamped to [minZoom,maxZoom]
// like ZoomAt), keeping the current viewport's *centre* point fixed --
// used by the Settings tab's zoom buttons, where there's no cursor
// position over the map to anchor to the way the mouse wheel's ZoomAt
// does.
func (c *Camera) SetZoom(scale float64, gridWidth, gridHeight int) {
	oldScale := c.Scale
	if oldScale <= 0 {
		oldScale = 1
	}
	newScale := clamp(scale, minZoom, maxZoom)
	if newScale == oldScale {
		return
	}

	centerX := c.viewportX + c.viewportWidth/2
	centerY := c.viewportY + c.viewportHeight/2
	worldX := c.X + float64(centerX-c.viewportX)/oldScale
	worldY := c.Y + float64(centerY-c.viewportY)/oldScale
	c.Scale = newScale
	c.X = worldX - float64(centerX-c.viewportX)/newScale
	c.Y = worldY - float64(centerY-c.viewportY)/newScale
	c.clamp(gridWidth, gridHeight)
}

// Pan moves the camera by (dx, dy) pixels and clamps it so the viewport
// never shows negative coordinates or scrolls past the map's far edge.
func (c *Camera) Pan(dx, dy float64, gridWidth, gridHeight, screenWidth, screenHeight int) {
	if c.Scale <= 0 {
		c.Scale = 1
	}
	c.X += dx / c.Scale
	c.Y += dy / c.Scale

	if c.viewportWidth > 0 {
		screenWidth = c.viewportWidth
	}
	if c.viewportHeight > 0 {
		screenHeight = c.viewportHeight
	}

	c.clampWithViewport(gridWidth, gridHeight, screenWidth, screenHeight)
}

func (c *Camera) clamp(gridWidth, gridHeight int) {
	c.clampWithViewport(gridWidth, gridHeight, c.viewportWidth, c.viewportHeight)
}

func (c *Camera) clampWithViewport(gridWidth, gridHeight, screenWidth, screenHeight int) {
	if c.Scale <= 0 {
		c.Scale = 1
	}
	worldViewportWidth := float64(screenWidth) / c.Scale
	worldViewportHeight := float64(screenHeight) / c.Scale
	mapWidth := float64(gridWidth * TileSize)
	mapHeight := float64(gridHeight * TileSize)
	maxX := mapWidth - worldViewportWidth
	maxY := mapHeight - worldViewportHeight
	if maxX < 0 {
		// A negative world-space offset creates equal empty margins around a
		// map that is smaller than its viewport. Keeping it at zero was what
		// pinned a zoomed-out map to the upper-left corner.
		c.X = maxX / 2
	} else {
		c.X = clamp(c.X, 0, maxX)
	}
	if maxY < 0 {
		c.Y = maxY / 2
	} else {
		c.Y = clamp(c.Y, 0, maxY)
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// TileToScreen returns the top-left screen pixel of tile (tx, ty) given
// the camera's current position.
func (c *Camera) TileToScreen(tx, ty int) (sx, sy float64) {
	return (float64(tx*TileSize)-c.X)*c.Scale + float64(c.viewportX), (float64(ty*TileSize)-c.Y)*c.Scale + float64(c.viewportY)
}

// ScreenToTile returns the grid coordinate under screen pixel (sx, sy).
func (c *Camera) ScreenToTile(sx, sy int) (tx, ty int) {
	if c.Scale <= 0 {
		c.Scale = 1
	}
	wx := float64(sx-c.viewportX)/c.Scale + c.X
	wy := float64(sy-c.viewportY)/c.Scale + c.Y
	return int(math.Floor(wx / TileSize)), int(math.Floor(wy / TileSize))
}

// VisibleTileBounds returns the tiles touched by the map viewport. margin
// expands the rectangle for sprites that rise above their own footprint or
// for a small prefetch ring while panning.
func (c *Camera) VisibleTileBounds(margin int) TileBounds {
	if margin < 0 {
		margin = 0
	}
	left, top := c.viewportX, c.viewportY
	width, height := c.viewportWidth, c.viewportHeight
	if width <= 0 || height <= 0 {
		left, top = 0, 0
		width, height = TileSize, TileSize
	}
	minX, minY := c.ScreenToTile(left, top)
	maxX, maxY := c.ScreenToTile(left+width, top+height)
	return TileBounds{
		MinX: minX - margin,
		MinY: minY - margin,
		MaxX: maxX + margin,
		MaxY: maxY + margin,
	}
}
