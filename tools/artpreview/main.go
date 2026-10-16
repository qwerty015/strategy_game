// artpreview renders a slice of the map with the real game renderer and saves
// it as PNG -- for judging new art (new_gen_game/) in context: north of the
// street are buildings with new Blender art, south of it legacy sprites.
//
//	go run ./tools/artpreview <out_prefix>   ->  <out_prefix>_zoom100.png, ...
//
// Needs a display (it runs ebiten.RunGame for a few frames, then exits).
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/building"
	"strategy_game/internal/render"
	"strategy_game/internal/world"
)

const w, h = 1400, 820

type app struct {
	grid      *world.Grid
	buildings []*building.Building
	cam       *render.Camera
	frame     int
	shots     []shot
}

type shot struct {
	zoom float64
	cx   float64
	cy   float64
	at   int
	path string
}

func (a *app) Update() error {
	render.Tick()
	a.frame++
	if len(a.shots) == 0 {
		return ebiten.Termination
	}
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	if len(a.shots) == 0 {
		return
	}
	s := a.shots[0]
	a.cam.SetViewport(0, 0, w, h)
	a.cam.SetZoom(s.zoom, a.grid.Width, a.grid.Height)
	a.cam.CenterOn(s.cx, s.cy, a.grid.Width, a.grid.Height)
	render.DrawGrid(screen, a.grid, a.cam)
	render.DrawBuildings(screen, a.grid, a.buildings, a.cam, map[*building.Building]bool{}, map[*building.Building]bool{}, nil)
	if a.frame >= s.at {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		screen.ReadPixels(img.Pix)
		f, err := os.Create(s.path)
		if err != nil {
			log.Fatal(err)
		}
		png.Encode(f, img)
		f.Close()
		fmt.Println("wrote", s.path)
		a.shots = a.shots[1:]
	}
}

func (a *app) Layout(int, int) (int, int) { return w, h }

func main() {
	out := os.Args[1]
	g := world.NewGrid(40, 24)
	var bs []*building.Building
	add := func(k building.Kind, x, y int) {
		bs = append(bs, &building.Building{Kind: k, X: x, Y: y, HP: building.MaxHP})
	}
	for x := 2; x < 38; x++ { // main street
		add(building.Road, x, 12)
	}
	// new Blender art on the street's north side, legacy art on the south side
	add(building.Warehouse, 6, 10)
	add(building.Mill, 11, 10)
	add(building.Bakery, 16, 10)
	add(building.Smeltery, 21, 10)
	add(building.Tavern, 26, 10)
	add(building.LumberjackHut, 31, 10)
	add(building.Barracks, 6, 15)
	add(building.CarpentryWorkshop, 11, 15)
	add(building.MeatWorkshop, 16, 15)
	add(building.Armory, 21, 15)
	add(building.QuarryHut, 26, 15)
	add(building.WatchTower, 31, 15)
	for y := 13; y < 15; y++ {
		add(building.Road, 6, y)
		add(building.Road, 16, y)
	}
	a := &app{grid: g, buildings: bs, cam: render.NewCamera(), shots: []shot{
		{zoom: 1.0, cx: 19, cy: 12, at: 40, path: out + "_zoom100.png"},
		{zoom: 2.0, cx: 14, cy: 11, at: 80, path: out + "_zoom200.png"},
		{zoom: 2.0, cx: 14, cy: 11, at: 83, path: out + "_zoom200_b.png"},
	}}
	ebiten.SetWindowSize(w, h)
	if err := ebiten.RunGame(a); err != nil {
		log.Fatal(err)
	}
}
