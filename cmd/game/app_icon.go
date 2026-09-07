package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

func setApplicationIcon() {
	exe, _ := os.Executable()
	candidates := []string{filepath.Join(filepath.Dir(exe), "assets", "app.png")}
	if _, source, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(source), "..", "..", "assets", "app.png"))
	}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err == nil {
			ebiten.SetWindowIcon([]image.Image{img})
			return
		}
	}
}
