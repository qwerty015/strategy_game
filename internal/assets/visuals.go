package assets

import (
	"image/color"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// LayerMode tells the renderer how a transparent image is attached to the
// world. A footprint layer fills a building's occupied square. A standing
// layer is anchored to one tile's ground contact point and may rise above it.
//
// Keeping this data with the assets instead of the simulation is deliberate:
// save files retain only building.Kind and construction progress. Visual packs
// may evolve independently without a save-file migration.
type LayerMode uint8

const (
	LayerStanding LayerMode = iota
	LayerFootprint
)

// VisualLayer is one renderable piece of a building. AnchorX/Y are tile offsets
// from the building's top-left cell. TilesTall controls the standing sprite's
// height; it is ignored by footprint layers. Tint is multiplied over the PNG,
// allowing a construction preview to reuse the building silhouette without
// baking a second copy into every fallback asset.
type VisualLayer struct {
	Image            *ebiten.Image
	Mode             LayerMode
	AnchorX, AnchorY float64
	TilesTall        float64
	Tint             color.RGBA
}

// BuildingVisual is the art contract for one building kind. Front is rendered
// in the foreground pass after units, so future porches, fences and roof eaves
// can correctly cover a unit that walks behind them. The current base sprites
// do not yet need a front slice, therefore Front is usually empty.
//
// There is deliberately no per-construction-stage art contract any more
// (no ConstructionVisual/Site/Preview) -- see render.drawConstructionSite,
// which instead fades Body itself in from translucent to opaque as
// building.Building.ConstructionProgress() advances. Per the user's
// explicit request: "если для какого-то здания есть спрайты этапов
// строительства, сразу используем прозрачное зданий/дорогу которая
// принимает видимость в процессе строительства" -- one real sprite per
// kind, faded, rather than a family of dedicated per-stage sprites.
type BuildingVisual struct {
	Body  VisualLayer
	Front VisualLayer
}

// BuildingVisualFor returns the visual definition for a gameplay building kind.
// A missing definition is intentional for flat roads and dynamic world objects
// such as trees/fish/deposits, which have their own state-based renderers.
func BuildingVisualFor(kind building.Kind) (BuildingVisual, bool) {
	visual, ok := buildingVisuals[kind]
	return visual, ok
}

// ResourceVisual reserves named art slots for every material currently known by
// the economy. The UI can use Icon; moving units use Carry; a future warehouse
// or ground pile can use Stack. These are filenames rather than decoded images
// so an incomplete optional art pack never prevents the game from starting.
type ResourceVisual struct {
	Icon  string
	Carry string
	Stack string
}

// ResourceVisualFor exposes the stable resource-art naming contract. All paths
// are relative to assets/sprites and are copied unchanged beside the executable.
func ResourceVisualFor(kind resource.Type) (ResourceVisual, bool) {
	visual, ok := resourceVisuals[kind]
	return visual, ok
}

// ResourceIcon, ResourceCarry and ResourceStack return decoded external PNGs
// for the interface, a worker's hands and future ground/storage piles. Nil is
// a safe result for a yet-unknown future resource type.
func ResourceIcon(kind resource.Type) *ebiten.Image  { return resourceSprites[kind].icon }
func ResourceCarry(kind resource.Type) *ebiten.Image { return resourceSprites[kind].carry }
func ResourceStack(kind resource.Type) *ebiten.Image { return resourceSprites[kind].stack }

type resourceSpriteSet struct {
	icon  *ebiten.Image
	carry *ebiten.Image
	stack *ebiten.Image
}

var resourceVisuals = map[resource.Type]ResourceVisual{
	resource.Wheat:        resourceVisual("wheat"),
	resource.Flour:        resourceVisual("flour"),
	resource.Bread:        resourceVisual("bread"),
	resource.Fish:         resourceVisual("fish"),
	resource.Wine:         resourceVisual("wine"),
	resource.Sausage:      resourceVisual("sausage"),
	resource.Carcass:      resourceVisual("carcass"),
	resource.Log:          resourceVisual("log"),
	resource.Plank:        resourceVisual("plank"),
	resource.StoneBlock:   resourceVisual("stone_block"),
	resource.Gold:         resourceVisual("gold"),
	resource.Coal:         resourceVisual("coal"),
	resource.GoldOre:      resourceVisual("gold_ore"),
	resource.IronOre:      resourceVisual("iron_ore"),
	resource.Iron:         resourceVisual("iron"),
	resource.Hide:         resourceVisual("hide"),
	resource.Bow:          resourceVisual("bow"),
	resource.LeatherArmor: resourceVisual("leather_armor"),
	resource.Sword:        resourceVisual("sword"),
}

var resourceSprites = func() map[resource.Type]resourceSpriteSet {
	images := make(map[resource.Type]resourceSpriteSet, len(resourceVisuals))
	for kind, visual := range resourceVisuals {
		images[kind] = resourceSpriteSet{
			icon:  loadNativeIfPresent(visual.Icon, nil),
			carry: loadNativeIfPresent(visual.Carry, nil),
			stack: loadNativeIfPresent(visual.Stack, nil),
		}
	}
	return images
}()

func resourceVisual(name string) ResourceVisual {
	root := "resources/" + name + "/"
	return ResourceVisual{
		Icon:  root + "icon.png",
		Carry: root + "carry.png",
		Stack: root + "stack.png",
	}
}

// The numbers below are visual extents, not logical placement footprints.
// A two-cell-looking roof makes the town legible at normal zoom while old
// saves, road access and collision rules retain their existing stable tiles.
var buildingVisuals = map[building.Kind]BuildingVisual{
	building.Road:              roadVisual(),
	building.Farm:              standardBuildingVisual("farm", FarmHouse, 1.95),
	building.Mill:              standardBuildingVisual("mill", MillFrames[0], 2.15),
	building.Bakery:            standardBuildingVisual("bakery", Bakery, 1.85),
	building.Warehouse:         standardBuildingVisual("warehouse", Warehouse, 2.15),
	building.Tavern:            standardBuildingVisual("tavern", Tavern, 2.00),
	building.LumberjackHut:     standardBuildingVisual("lumberjack_hut", LumberjackHut, 1.85),
	building.Winery:            standardBuildingVisual("winery", Winery, 1.95),
	building.FisherHut:         standardBuildingVisual("fisher_hut", FisherHutFrames[0], 1.85),
	building.PigFarm:           standardBuildingVisual("pig_farm", PigFarm, 1.95),
	building.MeatWorkshop:      standardBuildingVisual("meat_workshop", MeatWorkshop, 1.95),
	building.CarpentryWorkshop: standardBuildingVisual("carpentry_workshop", CarpentryWorkshop, 1.90),
	building.QuarryHut:         standardBuildingVisual("quarry_hut", QuarryHut, 1.90),
	building.MinerHut:          standardBuildingVisual("miner_hut", MinerHut, 1.90),
	building.Smeltery:          standardBuildingVisual("smeltery", Smeltery, 2.10),
	building.WatchTower:        standardBuildingVisual("watch_tower", WatchTower, 2.45),
	building.Barracks:          standardBuildingVisual("barracks", Barracks, 2.05),
	building.Armory:            standardBuildingVisual("armory", Armory, 2.00),
}

// roadVisual gives Road the same treatment as every other kind now (see
// standardBuildingVisual's doc comment): its Body IS the real, finished
// cobblestone tile (assets.Road), drawn in footprint mode since a road has
// no standing height -- render.drawConstructionSite fades this in exactly
// like a building's Body, per the user's explicit "и если для какого-то
// здания есть спрайты этапов строительства, сразу используем прозрачное
// зданий/дорогу которая принимает видимость в процессе строительства".
func roadVisual() BuildingVisual {
	return BuildingVisual{Body: footprintLayer(Road, color.RGBA{255, 255, 255, 255})}
}

// standardBuildingVisual builds the art contract for a building that has a
// normal standing Body sprite. It deliberately does not fill in any
// per-stage construction art: a building under construction fades its own
// finished Body sprite up from translucent to opaque as it progresses (see
// render.drawConstructionSite) instead of needing separate foundation/
// waiting/finishing art per building. That dedicated art was only ever
// finished for Farm before this change -- every other building already fell
// back to the shared generic site sheets, so nothing else loses real art;
// see AGENTS.md for the full reasoning.
func standardBuildingVisual(slug string, body *ebiten.Image, tilesTall float64) BuildingVisual {
	// body.png is optional during the migration. Once a dedicated natural-size
	// sprite is present it replaces the legacy 64px fallback automatically.
	body = loadNativeIfPresent(filepath.Join("buildings", slug, "body.png"), body)
	bodyLayer := standingLayer(body, tilesTall, color.RGBA{255, 255, 255, 255})

	return BuildingVisual{
		Body: bodyLayer,
		Front: standingLayer(
			loadNativeIfPresent(filepath.Join("buildings", slug, "front.png"), nil),
			tilesTall,
			color.RGBA{255, 255, 255, 255},
		),
	}
}

func standingLayer(image *ebiten.Image, tilesTall float64, tint color.RGBA) VisualLayer {
	return VisualLayer{Image: image, Mode: LayerStanding, TilesTall: tilesTall, Tint: tint}
}

func footprintLayer(image *ebiten.Image, tint color.RGBA) VisualLayer {
	return VisualLayer{Image: image, Mode: LayerFootprint, Tint: tint}
}

// loadNativeIfPresent keeps the runtime tolerant while an art pack is being
// built. Dedicated files retain their natural pixel dimensions and are placed
// by their layer metadata; absent files resolve to the current approved fallback.
func loadNativeIfPresent(name string, fallback *ebiten.Image) *ebiten.Image {
	path := filepath.Join(spriteDir, filepath.FromSlash(name))
	if _, err := os.Stat(path); err != nil {
		return fallback
	}
	return ebiten.NewImageFromImage(mustDecode(name))
}
