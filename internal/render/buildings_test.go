package render

import (
	"testing"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
)

// TestConstructionOpacityByte_FadesFromMinToFullyOpaque locks in the
// user's explicit request: "ассет имеет заливку (прозрачность) условно
// 30%... по мере строительство постройка постепенно становится более
// видимой плавно поднимаем до 100% видимости" -- progress 0 must read as
// constructionMinOpacity, progress 1 as fully opaque, and it must not dip
// or jump anywhere in between.
func TestConstructionOpacityByte_FadesFromMinToFullyOpaque(t *testing.T) {
	wantMin := constructionOpacityByte(0)
	if wantMin == 0 || wantMin == 255 {
		t.Fatalf("constructionOpacityByte(0) = %d, want a value strictly between 0 and 255 (constructionMinOpacity should be a partial fade, not 0%% or 100%%)", wantMin)
	}
	if got := constructionOpacityByte(0); got != wantMin {
		t.Errorf("constructionOpacityByte(0) = %d, want %d (constructionMinOpacity)", got, wantMin)
	}
	if got := constructionOpacityByte(1); got != 255 {
		t.Errorf("constructionOpacityByte(1) = %d, want 255 (fully opaque)", got)
	}
	if got := constructionOpacityByte(0.5); got <= wantMin || got == 255 {
		t.Errorf("constructionOpacityByte(0.5) = %d, want strictly between %d and 255", got, wantMin)
	}

	prev := constructionOpacityByte(0)
	for i := 1; i <= 10; i++ {
		got := constructionOpacityByte(float64(i) / 10)
		if got < prev {
			t.Fatalf("constructionOpacityByte(%.1f) = %d, dipped below the previous step's %d", float64(i)/10, got, prev)
		}
		prev = got
	}
}

// TestConstructionOpacityByte_ClampsOutOfRangeProgress guards against a
// caller passing an unclamped ConstructionProgress() edge case (or a
// future bug in it) from ever wrapping around uint8 instead of clamping.
func TestConstructionOpacityByte_ClampsOutOfRangeProgress(t *testing.T) {
	if got := constructionOpacityByte(-1); got != constructionOpacityByte(0) {
		t.Errorf("constructionOpacityByte(-1) = %d, want the same as progress 0", got)
	}
	if got := constructionOpacityByte(2); got != 255 {
		t.Errorf("constructionOpacityByte(2) = %d, want the same as progress 1", got)
	}
}

// TestWallPreviewArt_ReflectsNeighboursRegardlessOfTheirOwnConstructionStage
// is wallPreviewArt's whole reason to exist, separate from
// FinishedWallSegments: a run of wall drawn all at once should preview its
// real intended shape (straight run, corner, ...) even while every segment
// in it -- not just the one being drawn -- is still under construction. A
// lone wall segment with no neighbours yet must still resolve to SOME
// frame (a cap), not nil.
func TestWallPreviewArt_ReflectsNeighboursRegardlessOfTheirOwnConstructionStage(t *testing.T) {
	middle := &building.Building{Kind: building.StoneWall, X: 5, Y: 5, ConstructionStage: building.ConstructionFoundation}
	lone := wallPreviewArt(nil, middle)
	if lone == nil {
		t.Fatal("a wall segment with no neighbours at all resolved to a nil preview frame")
	}

	west := &building.Building{Kind: building.StoneWall, X: 4, Y: 5, ConstructionStage: building.ConstructionFoundation}
	east := &building.Building{Kind: building.StoneWall, X: 6, Y: 5, ConstructionStage: building.ConstructionWaitingMaterials}
	straight := wallPreviewArt([]*building.Building{west, middle, east}, middle)
	if straight == nil {
		t.Fatal("a straight run of walls (all still under construction) resolved to a nil preview frame")
	}
	if straight == lone {
		t.Fatal("wallPreviewArt ignored the two neighbouring wall segments -- straight-run frame must differ from the lone-cap frame")
	}
}

// TestWallPreviewArt_GateUsesItsOwnAxis covers the Gate half of
// wallPreviewArt -- it has no shape to resolve from neighbours (a gate
// segment is never part of a longer run the way a plain wall tile can be),
// just its own recorded axis.
func TestWallPreviewArt_GateUsesItsOwnAxis(t *testing.T) {
	horizontal := &building.Building{Kind: building.Gate, X: 1, Y: 1, GateAxis: building.WallHorizontal, ConstructionStage: building.ConstructionFoundation}
	if got := wallPreviewArt(nil, horizontal); got != assets.GateHorizontalClosed {
		t.Error("horizontal Gate preview did not resolve to GateHorizontalClosed")
	}
	vertical := &building.Building{Kind: building.Gate, X: 1, Y: 1, GateAxis: building.WallVertical, ConstructionStage: building.ConstructionFoundation}
	if got := wallPreviewArt(nil, vertical); got != assets.GateVerticalClosed {
		t.Error("vertical Gate preview did not resolve to GateVerticalClosed")
	}
}
