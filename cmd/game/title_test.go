package main

import "testing"

func TestParseHelpMarkdownSplitsPagesAndAssets(t *testing.T) {
	pages := parseHelpMarkdown(`# Document title

## First page

Plain **text**.

![Farm](../internal/assets/generated/building_farm.png)

## Second page

### Detail

- A list item.
`)
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	if pages[0].title != "First page" || len(pages[0].lines) != 2 {
		t.Fatalf("first page = %#v", pages[0])
	}
	if got := pages[0].lines[0]; got.kind != helpLineText || got.text != "Plain text." {
		t.Fatalf("first line = %#v", got)
	}
	if got := pages[0].lines[1]; got.kind != helpLineImage || got.image != "../internal/assets/generated/building_farm.png" {
		t.Fatalf("image line = %#v", got)
	}
	if pages[1].title != "Second page" || pages[1].lines[0].kind != helpLineHeading {
		t.Fatalf("second page = %#v", pages[1])
	}
}

func TestTitlePresentationUsesTwoHundredPercentZoom(t *testing.T) {
	if titleMapZoom != 2.00 {
		t.Fatalf("title map zoom = %.2f, want 2.00", titleMapZoom)
	}
}

func TestTitleActionAt(t *testing.T) {
	rects := titleButtonRects(1280, 720)
	for index, rect := range rects {
		action, ok := titleActionAt(rect.Min.X+1, rect.Min.Y+1, 1280, 720)
		if !ok || action != titleAction(index) {
			t.Fatalf("button %d: action=%d ok=%v", index, action, ok)
		}
	}
	if _, ok := titleActionAt(0, 0, 1280, 720); ok {
		t.Fatal("empty point unexpectedly has an action")
	}
}
