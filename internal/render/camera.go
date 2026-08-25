// Package render turns game state (world.Grid, buildings) into pixels on
// an ebiten screen. It's the only place besides cmd/game and ui that is
// allowed to depend on ebiten.
package render

const TileSize = 24 // pixels per tile side, at default zoom

// Camera tracks which part of the world (in pixels) is visible at the
// top-left of the screen, and converts between world and screen space.
type Camera struct {
	X, Y float64 // world-space pixel offset of the viewport's top-left

	viewportX, viewportY          int
	viewportWidth, viewportHeight int
}

// NewCamera creates a camera positioned at the world origin.
func NewCamera() *Camera {
	return &Camera{}
}

// SetViewport tells the camera where the map is drawn on the screen. UI
// panels can then occupy the edges without desynchronizing map placement.
func (c *Camera) SetViewport(x, y, width, height int) {
	c.viewportX, c.viewportY = x, y
	c.viewportWidth, c.viewportHeight = width, height
}

// Pan moves the camera by (dx, dy) pixels and clamps it so the viewport
// never shows negative coordinates or scrolls past the map's far edge.
func (c *Camera) Pan(dx, dy float64, gridWidth, gridHeight, screenWidth, screenHeight int) {
	c.X += dx
	c.Y += dy

	if c.viewportWidth > 0 {
		screenWidth = c.viewportWidth
	}
	if c.viewportHeight > 0 {
		screenHeight = c.viewportHeight
	}

	maxX := float64(gridWidth*TileSize - screenWidth)
	maxY := float64(gridHeight*TileSize - screenHeight)
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}

	c.X = clamp(c.X, 0, maxX)
	c.Y = clamp(c.Y, 0, maxY)
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
	return float64(tx*TileSize) - c.X + float64(c.viewportX), float64(ty*TileSize) - c.Y + float64(c.viewportY)
}

// ScreenToTile returns the grid coordinate under screen pixel (sx, sy).
func (c *Camera) ScreenToTile(sx, sy int) (tx, ty int) {
	wx := float64(sx-c.viewportX) + c.X
	wy := float64(sy-c.viewportY) + c.Y
	return int(wx) / TileSize, int(wy) / TileSize
}
