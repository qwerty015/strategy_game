// generate_wall_sprites turns the approved wall/gate concept sheet into the
// small transparent PNG files consumed by the external game art pack. Keeping
// the cutter in tools makes the resulting assets repeatable without putting
// source art into the executable.
package main

import (
	"image"
	"image/color"
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
	cellW, cellH := bounds.Dx()/4, bounds.Dy()/2
	if cellW <= 0 || cellH <= 0 {
		log.Fatal("wall sprite sheet must have four columns and two rows")
	}
	frames := []frame{
		{"buildings/wall/horizontal.png", image.Rect(0, 0, cellW, cellH)},
		{"buildings/wall/vertical.png", image.Rect(cellW, 0, cellW*2, cellH)},
		{"buildings/gate/horizontal_closed.png", image.Rect(cellW*2, 0, cellW*3, cellH)},
		{"buildings/gate/vertical_closed.png", image.Rect(cellW*3, 0, cellW*4, cellH)},
		{"buildings/gate/horizontal_open.png", image.Rect(0, cellH, cellW, cellH*2)},
		{"buildings/gate/vertical_open.png", image.Rect(cellW, cellH, cellW*2, cellH*2)},
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
	stripBlack(crop)
	content, ok := opaqueBounds(crop)
	if !ok {
		return image.NewNRGBA(image.Rect(0, 0, 64, 64))
	}
	const pad = 2
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
	scale := math.Min(60/float64(content.Dx()), 60/float64(content.Dy()))
	width := int(math.Round(float64(content.Dx()) * scale))
	height := int(math.Round(float64(content.Dy()) * scale))
	for y := 0; y < height; y++ {
		sy := content.Min.Y + y*content.Dy()/height
		for x := 0; x < width; x++ {
			sx := content.Min.X + x*content.Dx()/width
			out.SetNRGBA((64-width)/2+x, (64-height)/2+y, crop.NRGBAAt(sx, sy))
		}
	}
	return out
}

func stripBlack(img *image.NRGBA) {
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			offset := img.PixOffset(x, y)
			r, g, b := img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2]
			if r < 12 && g < 12 && b < 12 {
				img.Pix[offset+3] = 0
			}
		}
	}
}

func opaqueBounds(img *image.NRGBA) (image.Rectangle, bool) {
	minX, minY := img.Bounds().Dx(), img.Bounds().Dy()
	maxX, maxY := -1, -1
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
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

var _ color.Color
