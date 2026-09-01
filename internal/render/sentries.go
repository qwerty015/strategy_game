package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/sentry"
)

// sentryHeight is slightly below a full tile so a guard walking to a Tavern
// reads as a person on the road rather than a second building.
const sentryHeight = 0.90

// DrawSentries renders a guard atop a finished tower while on duty, or as an
// ordinary three-frame walking unit while travelling to and from a Tavern.
// The simulation state remains in package sentry; this file intentionally
// derives only visual posture and tint from its public read-only methods.
func DrawSentries(screen *ebiten.Image, sentries []*sentry.Sentry, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(2)
	for _, guard := range sentries {
		if guard == nil || !visible.Intersects(guard.X, guard.Y, 1) {
			continue
		}

		sx, sy := cam.TileToScreen(guard.X, guard.Y)
		path := guard.RemainingPath()
		frame := walkingFrame(path, guard.X+guard.Y)
		bob := float64(0)
		if guard.Working() {
			// Anchor the idle sentry inside the upper parapet instead of at the
			// tower door. The number is visual-only and independent of the
			// building's logical one-tile footprint.
			sy -= 1.15 * tilePixels
			frame = 1
		} else {
			bob = unitBob()
		}

		tint := color.Color(color.White)
		if guard.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		flip := facingLeft(guard.X, path)
		drawStandingFacingTintedAtScale(screen, assets.SentryWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, sentryHeight, tilePixels, tint, flip)
	}
}
