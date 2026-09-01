// generate_wall_sprites turns the approved wall/gate concept sheet into the
// transparent PNG tiles consumed by the external game art pack. The source
// sheet is intentionally not embedded: only compact, final game sprites live
// in the repository.
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
	frames := []frame{
		{"buildings/wall/horizontal.png", cell(0, 0)},
		{"buildings/wall/vertical.png", cell(1, 0)},
		{"buildings/wall/corner_ne.png", cell(0, 1)},
		{"buildings/wall/corner_nw.png", cell(1, 1)},
		{"buildings/wall/corner_se.png", cell(2, 0)},
		{"buildings/wall/corner_sw.png", cell(3, 0)},
		{"buildings/wall/tee.png", cell(2, 2)},
		{"buildings/wall/cross.png", cell(3, 2)},
		{"buildings/wall/pillar.png", cell(0, 3)},
		{"buildings/gate/horizontal_closed.png", cell(1, 3)},
		{"buildings/gate/horizontal_open.png", cell(2, 3)},
		{"buildings/gate/vertical_closed.png", image.Rect(3*cellW, 3*cellH, 3*cellW+cellW/2, 4*cellH)},
		{"buildings/gate/vertical_open.png", image.Rect(3*cellW+cellW/2, 3*cellH, 4*cellW, 4*cellH)},
	}
	for _, f := range frames {
		out := normalizedFrame(sheet, f.cell)
		path := filepath.Join(os.Args[2], filepath.FromSlash(f.name))
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
	if maxX < minX || maxY < minY {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}
