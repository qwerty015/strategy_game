// generate_wall_sprites turns the approved wall/gate concept sheet into the
// transparent PNG tiles consumed by the external game art pack. Corners are
// composited from the same straight horizontal/vertical modules: this keeps
// stone scale, height and edge positions identical at every join.
package main

import (
	"image"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

type frame struct {
	name string
	cell image.Rectangle
}

type wallModule struct {
	name               string
	north, east, south bool
	west               bool
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: go run tools/generate_wall_sprites.go <sheet.png> <sprite-root>")
	}
	source, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()
	sheet, err := png.Decode(source)
	if err != nil {
		log.Fatal(err)
	}
	bounds := sheet.Bounds()
	cellW, cellH := bounds.Dx()/4, bounds.Dy()/4
	if cellW <= 0 || cellH <= 0 {
		log.Fatal("wall sprite sheet must have four columns and four rows")
	}
	cell := func(col, row int) image.Rectangle {
		return image.Rect(col*cellW, row*cellH, (col+1)*cellW, (row+1)*cellH)
	}

	// A raw concept cell includes a large post on every edge. Keep only its
	// masonry centre and stretch that texture along the axis: adjacent wall
	// tiles now meet as one uninterrupted run instead of a chain of fences.
	horizontal := stretchHorizontal(normalizedFrame(sheet, cell(0, 0)), 15, 49)
	vertical := stretchVertical(normalizedFrame(sheet, cell(1, 0)), 15, 49)
	writeFrame(os.Args[2], "buildings/wall/horizontal.png", horizontal)
	writeFrame(os.Args[2], "buildings/wall/vertical.png", vertical)
	writeFrame(os.Args[2], "buildings/wall/pillar.png", normalizedFrame(sheet, cell(0, 3)))

	// Every branch/corner is built from the two straight pieces rather than
	// independently fitted concept cells. Their arms therefore meet in the
	// exact same centre line as a normal continuation.
	for _, module := range []wallModule{
		{"buildings/wall/corner_ne.png", true, true, false, false},
		{"buildings/wall/corner_nw.png", true, false, false, true},
		{"buildings/wall/corner_se.png", false, true, true, false},
		{"buildings/wall/corner_sw.png", false, false, true, true},
	} {
		writeFrame(os.Args[2], module.name, composeWall(horizontal, vertical, module))
	}

	frames := []frame{
		{"buildings/gate/horizontal_closed.png", cell(1, 3)},
		{"buildings/gate/horizontal_open.png", cell(2, 3)},
		{"buildings/gate/vertical_closed.png", image.Rect(3*cellW, 3*cellH, 3*cellW+cellW/2, 4*cellH)},
		{"buildings/gate/vertical_open.png", image.Rect(3*cellW+cellW/2, 3*cellH, 4*cellW, 4*cellH)},
	}
	for _, f := range frames {
		writeFrame(os.Args[2], f.name, normalizedFrame(sheet, f.cell))
	}
}

// stretchHorizontal repeats the central masonry column across a full tile,
// deliberately dropping the standalone end posts from the source concept.
func stretchHorizontal(src *image.NRGBA, minX, maxX int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	width := maxX - minX
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			sx := minX + x*width/64
			out.SetNRGBA(x, y, src.NRGBAAt(sx, y))
		}
	}
	return out
}

// stretchVertical is stretchHorizontal's 90-degree counterpart.
func stretchVertical(src *image.NRGBA, minY, maxY int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	height := maxY - minY
	for y := 0; y < 64; y++ {
		sy := minY + y*height/64
		for x := 0; x < 64; x++ {
			out.SetNRGBA(x, y, src.NRGBAAt(x, sy))
		}
	}
	return out
}
func writeFrame(root, name string, out image.Image) {
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(file, out); err != nil {
		file.Close()
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
}

// composeWall makes a compact, mitred junction from the same normalised
// straight sprites. Each arm begins one quarter into the tile, so the centre
// is reinforced by stone but does not turn into a bulky square tower.
func composeWall(horizontal, vertical *image.NRGBA, module wallModule) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	const joinInset = 24
	const joinEnd = 64 - joinInset
	if module.west {
		copyPart(out, horizontal, image.Rect(0, 0, joinEnd, 64))
	}
	if module.east {
		copyPart(out, horizontal, image.Rect(joinInset, 0, 64, 64))
	}
	if module.north {
		copyPart(out, vertical, image.Rect(0, 0, 64, joinEnd))
	}
	if module.south {
		copyPart(out, vertical, image.Rect(0, joinInset, 64, 64))
	}
	return out
}

func copyPart(dst, src *image.NRGBA, area image.Rectangle) {
	draw.Draw(dst, area, src, area.Min, draw.Over)
}

func normalizedFrame(sheet image.Image, cell image.Rectangle) *image.NRGBA {
	crop := image.NewNRGBA(image.Rect(0, 0, cell.Dx(), cell.Dy()))
	draw.Draw(crop, crop.Bounds(), sheet, cell.Min, draw.Src)
	stripExportBackdrop(crop)
	content, ok := opaqueBounds(crop)
	if !ok {
		return image.NewNRGBA(image.Rect(0, 0, 64, 64))
	}
	const pad = 3
	content = content.Inset(-pad)
	if content.Min.X < 0 {
		content.Min.X = 0
	}
	if content.Min.Y < 0 {
		content.Min.Y = 0
	}
	if content.Max.X > crop.Bounds().Dx() {
		content.Max.X = crop.Bounds().Dx()
	}
	if content.Max.Y > crop.Bounds().Dy() {
		content.Max.Y = crop.Bounds().Dy()
	}
	out := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	scale := math.Min(62/float64(content.Dx()), 62/float64(content.Dy()))
	width := max(1, int(math.Round(float64(content.Dx())*scale)))
	height := max(1, int(math.Round(float64(content.Dy())*scale)))
	for y := 0; y < height; y++ {
		sy := content.Min.Y + y*content.Dy()/height
		for x := 0; x < width; x++ {
			sx := content.Min.X + x*content.Dx()/width
			out.SetNRGBA((64-width)/2+x, (64-height)/2+y, crop.NRGBAAt(sx, sy))
		}
	}
	return out
}

// stripExportBackdrop removes the black studio background and tiny red/yellow
// mattes left by the image export. None of those colours belong to stone; the
// gate wood is brown and remains untouched.
func stripExportBackdrop(img *image.NRGBA) {
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			offset := img.PixOffset(x, y)
			r, g, b := img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2]
			if (r < 16 && g < 16 && b < 16) ||
				(r > 170 && g < 85 && b < 85) ||
				(r > 210 && g > 180 && b < 95) {
				img.Pix[offset+3] = 0
			}
		}
	}
}

func opaqueBounds(img *image.NRGBA) (image.Rectangle, bool) {
	minX, minY := img.Bounds().Dx(), img.Bounds().Dy()
	maxX, maxY := -1, -1
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.NRGBAAt(x, y).A <= 8 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}
