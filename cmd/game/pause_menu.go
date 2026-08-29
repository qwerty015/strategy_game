package main

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	gamehelp "strategy_game"
	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/ui"
)

// pauseMenuLayout keeps the Esc menu's drawing and hit areas together. It is
// intentionally independent of the side panels: the pause menu stays centered
// and usable at every supported window size.
type pauseMenuLayout struct {
	panel image.Rectangle

	resume image.Rectangle
	help   image.Rectangle
	new    image.Rectangle
	exit   image.Rectangle

	languages [2]image.Rectangle
	speeds    []image.Rectangle
	slots     []pauseSlotRects

	modal        image.Rectangle
	modalConfirm image.Rectangle
	modalCancel  image.Rectangle
	modalField   image.Rectangle
}

type pauseSlotRects struct {
	autosave image.Rectangle
	save     image.Rectangle
	load     image.Rectangle
	nameY    int
}

func newPauseMenuLayout(width, height int) pauseMenuLayout {
	panelW, panelH := 700, 650
	if limit := width - 40; panelW > limit {
		panelW = limit
	}
	if limit := height - 36; panelH > limit {
		panelH = limit
	}
	if panelW < 320 {
		panelW = 320
	}
	if panelH < 500 {
		panelH = 500
	}
	panel := image.Rect((width-panelW)/2, (height-panelH)/2, (width+panelW)/2, (height+panelH)/2)
	contentX, contentW := panel.Min.X+24, panel.Dx()-48
	buttonW := (contentW - 10) / 2
	button := func(x, y, w int) image.Rectangle { return image.Rect(x, y, x+w, y+34) }
	topY := panel.Min.Y + 54

	layout := pauseMenuLayout{
		panel:  panel,
		resume: button(contentX, topY, buttonW),
		help:   button(contentX+buttonW+10, topY, buttonW),
		new:    button(contentX, topY+42, buttonW),
		exit:   button(contentX+buttonW+10, topY+42, buttonW),
	}
	languageY := topY + 94
	layout.languages[0] = image.Rect(contentX, languageY, contentX+buttonW, languageY+28)
	layout.languages[1] = image.Rect(contentX+buttonW+10, languageY, contentX+contentW, languageY+28)

	speedY := languageY + 48
	speedW := contentW / (int(economy.Sixteenfold) + 1)
	layout.speeds = make([]image.Rectangle, int(economy.Sixteenfold)+1)
	for index := range layout.speeds {
		x := contentX + index*speedW
		w := speedW - 2
		if index == len(layout.speeds)-1 {
			w = contentX + contentW - x
		}
		layout.speeds[index] = image.Rect(x, speedY, x+w, speedY+30)
	}

	slotsY := speedY + 64
	layout.slots = make([]pauseSlotRects, slotCount)
	for index := range layout.slots {
		y := slotsY + index*60
		autoW := 54
		layout.slots[index] = pauseSlotRects{
			autosave: image.Rect(contentX+contentW-autoW, y, contentX+contentW, y+22),
			save:     image.Rect(contentX, y+25, contentX+buttonW, y+53),
			load:     image.Rect(contentX+buttonW+10, y+25, contentX+contentW, y+53),
			nameY:    y,
		}
	}

	modalW, modalH := 480, 210
	if modalW > panel.Dx()-40 {
		modalW = panel.Dx() - 40
	}
	modal := image.Rect((width-modalW)/2, (height-modalH)/2, (width+modalW)/2, (height+modalH)/2)
	modalButtonW := (modal.Dx() - 54) / 2
	layout.modal = modal
	layout.modalField = image.Rect(modal.Min.X+20, modal.Min.Y+86, modal.Max.X-20, modal.Min.Y+118)
	layout.modalConfirm = image.Rect(modal.Min.X+20, modal.Max.Y-54, modal.Min.X+20+modalButtonW, modal.Max.Y-22)
	layout.modalCancel = image.Rect(modal.Min.X+34+modalButtonW, modal.Max.Y-54, modal.Max.X-20, modal.Max.Y-22)
	return layout
}

func (g *Game) openPauseMenu() {
	g.paused = true
	g.pauseHelp = false
	g.middlePanning = false
	g.leftPanning = false
	g.leftPanMoved = false
}

func (g *Game) closePauseMenu() {
	g.paused = false
	g.pauseHelp = false
	g.middlePanning = false
	g.leftPanning = false
	g.leftPanMoved = false
}

// updatePauseMenu owns every input while the game is paused. No camera input,
// placement click or simulation tick can leak through this branch.
func (g *Game) updatePauseMenu() error {
	if g.dialog != ui.DialogNone {
		return g.handlePauseDialogInput()
	}
	if g.pauseHelp {
		return g.updatePauseHelp()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closePauseMenu()
		return nil
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}

	mx, my := ebiten.CursorPosition()
	point := image.Pt(mx, my)
	layout := newPauseMenuLayout(g.layout.Width, g.layout.Height)
	switch {
	case point.In(layout.resume):
		g.closePauseMenu()
	case point.In(layout.help):
		if len(g.helpPages) == 0 {
			g.helpPages = parseHelpMarkdown(gamehelp.HelpMarkdown)
		}
		g.helpPage = 0
		g.pauseHelp = true
	case point.In(layout.new):
		g.dialog = ui.DialogConfirmNewGame
	case point.In(layout.exit):
		g.dialog = ui.DialogConfirmExit
	case point.In(layout.languages[0]):
		i18n.SetLang(i18n.RU)
	case point.In(layout.languages[1]):
		i18n.SetLang(i18n.EN)
	default:
		for index, rect := range layout.speeds {
			if point.In(rect) {
				g.sim.SetSpeed(economy.Speed(index))
				return nil
			}
		}
		for index, slot := range layout.slots {
			slotNumber := index + 1
			switch {
			case point.In(slot.autosave):
				g.toggleAutosaveSlot(slotNumber)
				return nil
			case point.In(slot.save):
				g.handleSaveSlotAction(slotNumber, ui.SaveSlotSave)
				return nil
			case point.In(slot.load) && index < len(g.slotCache) && g.slotCache[index].Occupied:
				g.handleSaveSlotAction(slotNumber, ui.SaveSlotLoad)
				return nil
			}
		}
	}
	return nil
}

func (g *Game) updatePauseHelp() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.pauseHelp = false
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) && g.helpPage > 0 {
		g.helpPage--
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) && g.helpPage+1 < len(g.helpPages) {
		g.helpPage++
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	mx, my := ebiten.CursorPosition()
	point := image.Pt(mx, my)
	back, previous, next := helpNavRects(g.layout.Width, g.layout.Height)
	switch {
	case point.In(back):
		g.pauseHelp = false
	case point.In(previous) && g.helpPage > 0:
		g.helpPage--
	case point.In(next) && g.helpPage+1 < len(g.helpPages):
		g.helpPage++
	}
	return nil
}

func (g *Game) drawPauseMenu(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, float32(g.layout.Width), float32(g.layout.Height), color.RGBA{R: 12, G: 10, B: 9, A: 172}, false)
	if g.pauseHelp {
		g.drawHelpScreen(screen)
		return
	}
	layout := newPauseMenuLayout(g.layout.Width, g.layout.Height)
	drawTitlePanel(screen, layout.panel)
	t := i18n.T()
	ui.DrawTitleText(screen, t.PauseMenuTitle, float64(layout.panel.Min.X+24), float64(layout.panel.Min.Y+20), 2)

	drawTitleButton(screen, layout.resume, t.ResumeButton, false)
	drawTitleButton(screen, layout.help, t.HelpButton, false)
	drawTitleButton(screen, layout.new, t.NewGameButton, false)
	drawTitleButton(screen, layout.exit, t.ExitButton, false)

	languageY := layout.languages[0].Min.Y - 16
	ui.DrawMenuText(screen, t.LanguageLabel, float64(layout.languages[0].Min.X), float64(languageY))
	drawPauseChoice(screen, layout.languages[0], "Русский", i18n.Current() == i18n.RU)
	drawPauseChoice(screen, layout.languages[1], "English", i18n.Current() == i18n.EN)

	speedY := layout.speeds[0].Min.Y - 16
	ui.DrawMenuText(screen, t.SpeedTitle, float64(layout.speeds[0].Min.X), float64(speedY))
	speedLabels := []string{t.SpeedPaused, t.SpeedHalf, t.SpeedNormal, t.SpeedDouble, t.SpeedQuadruple, t.SpeedOctuple, t.SpeedSixteenfold}
	for index, rect := range layout.speeds {
		drawPauseChoice(screen, rect, speedLabels[index], economy.Speed(index) == g.sim.Speed())
	}

	ui.DrawMenuText(screen, t.SaveSlotsLabel, float64(layout.slots[0].save.Min.X), float64(layout.slots[0].nameY-16))
	for index, slot := range layout.slots {
		info := ui.SaveSlotInfo{}
		if index < len(g.slotCache) {
			info = g.slotCache[index]
		}
		name := info.Name
		if !info.Occupied {
			name = t.SlotEmptyLabel
		}
		ui.DrawMenuText(screen, fmt.Sprintf("%d. %s", index+1, name), float64(slot.save.Min.X), float64(slot.nameY))
		drawPauseChoice(screen, slot.autosave, t.AutosaveToggle, info.Autosave)
		drawPauseChoice(screen, slot.save, t.SlotSaveButton, false)
		drawPauseChoice(screen, slot.load, t.SlotLoadButton, false, !info.Occupied)
	}
	if g.dialog != ui.DialogNone {
		g.drawPauseDialog(screen, layout)
	}
}

func drawPauseChoice(screen *ebiten.Image, rect image.Rectangle, label string, selected bool, disabled ...bool) {
	muted := len(disabled) > 0 && disabled[0]
	fill := color.RGBA{R: 73, G: 57, B: 48, A: 245}
	border := color.RGBA{R: 166, G: 117, B: 58, A: 255}
	if selected {
		fill = color.RGBA{R: 177, G: 123, B: 46, A: 250}
	}
	if muted {
		fill = color.RGBA{R: 52, G: 44, B: 40, A: 230}
		border = color.RGBA{R: 89, G: 71, B: 54, A: 220}
	}
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), fill, false)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, border, false)
	ui.DrawCompactMenuText(screen, label, float64(rect.Min.X+6), float64(rect.Min.Y+7))
}

func (g *Game) drawPauseDialog(screen *ebiten.Image, layout pauseMenuLayout) {
	vector.FillRect(screen, 0, 0, float32(g.layout.Width), float32(g.layout.Height), color.RGBA{R: 7, G: 6, B: 6, A: 84}, false)
	drawTitlePanel(screen, layout.modal)
	t := i18n.T()
	prompt := ""
	confirmLabel := t.SlotOverwriteButton
	switch g.dialog {
	case ui.DialogConfirmOverwrite:
		prompt = fmt.Sprintf(t.SlotOverwritePrompt, g.dialogSlot, g.dialogText)
	case ui.DialogNaming:
		prompt = fmt.Sprintf(t.SlotNamePrompt, g.dialogSlot)
		confirmLabel = t.SlotSaveButton
		vector.FillRect(screen, float32(layout.modalField.Min.X), float32(layout.modalField.Min.Y), float32(layout.modalField.Dx()), float32(layout.modalField.Dy()), color.RGBA{R: 55, G: 45, B: 39, A: 255}, false)
		ui.DrawTitleText(screen, g.dialogText+"_", float64(layout.modalField.Min.X+8), float64(layout.modalField.Min.Y+8), 1.15)
	case ui.DialogConfirmNewGame:
		prompt = t.NewGameConfirmPrompt
		confirmLabel = t.NewGameConfirmButton
	case ui.DialogConfirmExit:
		prompt = t.ExitConfirmPrompt
		confirmLabel = t.ExitConfirmButton
	}
	ui.DrawTitleText(screen, prompt, float64(layout.modal.Min.X+20), float64(layout.modal.Min.Y+28), 1.15)
	drawTitleButton(screen, layout.modalConfirm, confirmLabel, false)
	drawTitleButton(screen, layout.modalCancel, t.SlotCancelButton, false)
}

func (g *Game) handlePauseDialogInput() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return nil
	}
	confirm := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	cancel := false
	layout := newPauseMenuLayout(g.layout.Width, g.layout.Height)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		point := image.Pt(mx, my)
		switch {
		case point.In(layout.modalConfirm):
			confirm = true
		case point.In(layout.modalCancel):
			cancel = true
		}
	}
	if cancel {
		g.dialog = ui.DialogNone
		return nil
	}

	if g.dialog == ui.DialogNaming {
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			if runes := []rune(g.dialogText); len(runes) > 0 {
				g.dialogText = string(runes[:len(runes)-1])
			}
		}
		for _, ch := range ebiten.AppendInputChars(nil) {
			if ch < ' ' || len([]rune(g.dialogText)) >= maxSlotNameLen {
				continue
			}
			g.dialogText += string(ch)
		}
		if confirm {
			g.commitDialogSave()
		}
		return nil
	}

	if !confirm {
		return nil
	}
	switch g.dialog {
	case ui.DialogConfirmOverwrite:
		g.dialog = ui.DialogNaming
	case ui.DialogConfirmNewGame:
		g.resetToNewGame()
	case ui.DialogConfirmExit:
		return ebiten.Termination
	}
	return nil
}
