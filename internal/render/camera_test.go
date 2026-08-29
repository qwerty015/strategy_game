package render

import "testing"

// TestCamera_SetZoom_KeepsViewportCenterFixed is a regression guard for
// the Settings tab's zoom buttons (see ui.SettingsZoomAt): unlike ZoomAt,
// which anchors to the cursor, a button click has no meaningful cursor
// position over the map to anchor to, so SetZoom must anchor to the
// viewport's own centre instead -- the world point under the middle of
// the screen should be the same tile before and after zooming.
func TestCamera_SetZoom_KeepsViewportCenterFixed(t *testing.T) {
	c := NewCamera()
	c.SetViewport(0, 0, 800, 600)
	c.X, c.Y = 500, 300

	centerX, centerY := 400, 300 // viewport centre in screen space
	beforeWX := c.X + float64(centerX)/c.Scale
	beforeWY := c.Y + float64(centerY)/c.Scale

	c.SetZoom(2.0, 200, 150)

	afterWX := c.X + float64(centerX)/c.Scale
	afterWY := c.Y + float64(centerY)/c.Scale
	if diff := beforeWX - afterWX; diff > 0.01 || diff < -0.01 {
		t.Errorf("world X under viewport centre = %.2f, want %.2f (unchanged)", afterWX, beforeWX)
	}
	if diff := beforeWY - afterWY; diff > 0.01 || diff < -0.01 {
		t.Errorf("world Y under viewport centre = %.2f, want %.2f (unchanged)", afterWY, beforeWY)
	}
	if c.Scale != 2.0 {
		t.Errorf("Scale = %.2f, want 2.0", c.Scale)
	}
}

// TestCamera_SetZoom_ClampsToRange covers the same [minZoom,maxZoom]
// bound ZoomAt already respects, so a Settings-tab preset can never push
// the camera past what the mouse wheel itself allows.
func TestCamera_SetZoom_ClampsToRange(t *testing.T) {
	c := NewCamera()
	c.SetViewport(0, 0, 800, 600)

	c.SetZoom(100, 200, 150)
	if c.Scale != maxZoom {
		t.Errorf("Scale after requesting 100x = %.2f, want clamped to maxZoom (%.2f)", c.Scale, maxZoom)
	}

	c.SetZoom(0.001, 200, 150)
	if c.Scale != minZoom {
		t.Errorf("Scale after requesting 0.001x = %.2f, want clamped to minZoom (%.2f)", c.Scale, minZoom)
	}
}
