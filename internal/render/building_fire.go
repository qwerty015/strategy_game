package render

import (
	"github.com/hajimehoshi/ebiten/v2"
	"strategy_game/internal/assets"
	"strategy_game/internal/building"
)

// Purely visual; no new damage, entities, timers or saved state.
func buildingFireIntensity(b *building.Building) float64 {
	if b == nil || b.ConstructionStage != building.ConstructionNone || b.HP <= 0 || b.HP >= building.MaxHP {
		return 0
	}
	switch b.Kind {
	case building.Road, building.Tree, building.Fish, building.StoneDeposit,
		building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		return 0
	}
	return float64(building.MaxHP-b.HP) / building.MaxHP
}

func drawBuildingDamageFire(screen *ebiten.Image, b *building.Building, sx, sy, tilePixels float64) {
	intensity := buildingFireIntensity(b)
	if intensity == 0 {
		return
	}
	// Fixed, staggered roof/wall anchors; never cover a farm's entire field.
	anchors := [5][2]float64{{0.5, 0.4}, {0.18, 0.65}, {0.79, 0.55}, {0.35, 0.0}, {0.67, -0.1}}
	count := 1 + int(intensity*4)
	for i := 0; i < count; i++ {
		art := assets.BuildingFireFrames[(animFrame/7+b.X*3+b.Y*5+i)%3]
		size := tilePixels * (0.4 + intensity*0.85)
		anchor := anchors[i]
		if building.IsWallKind(b.Kind) {
			size *= 0.65
			anchor[1] = 0.65
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(size/float64(art.Bounds().Dx()), size/float64(art.Bounds().Dy()))
		op.GeoM.Translate(sx+anchor[0]*tilePixels-size/2, sy+anchor[1]*tilePixels-size*0.9)
		op.ColorScale.ScaleAlpha(float32(0.7 + intensity*0.3))
		screen.DrawImage(art, op)
	}
}
