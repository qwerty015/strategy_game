package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/building"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
)

// DrawBufferLevels labels every production building with a compact
// "in→out" readout of its buffers (e.g. "3→5"), so the player can see
// where the economy is backing up without opening anything -- the
// "monitor what's sitting in each building" the user asked for.
// Warehouse/Road, which have no buffers of their own, are skipped.
func DrawBufferLevels(screen *ebiten.Image, buildings []*building.Building, cam *render.Camera) {
	for _, b := range buildings {
		bt := building.Types[b.Kind]
		hasInputs := len(bt.Recipe.Inputs) > 0
		hasOutput := bt.Recipe.TicksToProduce > 0
		isLumberjackHut := b.Kind == building.LumberjackHut
		if !hasInputs && !hasOutput && !isLumberjackHut {
			continue
		}

		sx, sy := cam.TileToScreen(b.X, b.Y)

		var line string
		switch {
		case hasInputs && hasOutput:
			line = fmt.Sprintf("%d→%d", bufferTotal(b.InputBuffer), bufferTotal(b.OutputBuffer))
		case hasInputs: // Tavern: draws down its accepted food inputs
			line = fmt.Sprintf("%d", bufferTotal(b.InputBuffer))
		case isLumberjackHut:
			line = fmt.Sprintf("%d", bufferTotal(b.OutputBuffer))
		default: // land producers: only fill their output buffer
			line = fmt.Sprintf("%d", bufferTotal(b.OutputBuffer))
		}
		DrawText(screen, line, sx, sy-14)
	}
}

func bufferTotal(m map[resource.Type]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}
