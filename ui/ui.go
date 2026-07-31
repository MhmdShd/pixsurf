// Package ui renders cell grids to the terminal and reports input events.
package ui

import (
	"sync"

	"github.com/gdamore/tcell/v2"

	"github.com/MhmdShd/pixsurf/cell"
)

// ActionKind identifies a navigation action.
type ActionKind int

const (
	ScrollUp ActionKind = iota
	ScrollDown
	PageUp
	PageDown
	Back
	Forward
	Reload
	Quit
	URLBar
)

// Event is one of ActionEvent, ClickEvent, ResizeEvent, InputEvent, InputCancelEvent.
type Event interface{}

// ActionEvent is a keyboard navigation action.
type ActionEvent struct{ Kind ActionKind }

// ClickEvent is a mouse click on grid cell (X, Y).
type ClickEvent struct{ X, Y int }

// ResizeEvent reports that the terminal was resized (also used to request redraws).
type ResizeEvent struct{}

// InputEvent is emitted when a status-bar prompt is submitted with Enter.
type InputEvent struct{ Text string }

// InputCancelEvent is emitted when a status-bar prompt is dismissed with Esc.
type InputCancelEvent struct{}

// UI owns the tcell screen.
type UI struct {
	screen tcell.Screen
	events chan Event

	mu        sync.Mutex
	inputMode bool
	label     string
	input     []rune
}

// New initializes the terminal and starts the input loop.
func New() (*UI, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnableMouse()
	u := &UI{screen: s, events: make(chan Event, 16)}
	go u.loop()
	return u, nil
}

// Events returns the input event channel.
func (u *UI) Events() <-chan Event { return u.events }

// GridSize returns the pixel-grid size: full width, height minus status bar.
func (u *UI) GridSize() (cols, rows int) {
	w, h := u.screen.Size()
	return w, h - 1
}

// Close restores the terminal.
func (u *UI) Close() { u.screen.Fini() }

func (u *UI) loop() {
	for {
		ev := u.screen.PollEvent()
		if ev == nil {
			return
		}
		switch e := ev.(type) {
		case *tcell.EventResize:
			u.screen.Sync()
			u.events <- ResizeEvent{}
		case *tcell.EventMouse:
			u.mu.Lock()
			prompting := u.inputMode
			u.mu.Unlock()
			if !prompting && e.Buttons()&tcell.Button1 != 0 {
				x, y := e.Position()
				_, rows := u.GridSize()
				if y < rows {
					u.events <- ClickEvent{X: x, Y: y}
				}
			}
		case *tcell.EventKey:
			u.handleKey(e)
		}
	}
}

func (u *UI) handleKey(e *tcell.EventKey) {
	u.mu.Lock()
	inInputMode := u.inputMode
	u.mu.Unlock()

	if inInputMode {
		u.handleInputKey(e)
		return
	}

	switch e.Key() {
	case tcell.KeyUp:
		u.events <- ActionEvent{Kind: ScrollUp}
		return
	case tcell.KeyDown:
		u.events <- ActionEvent{Kind: ScrollDown}
		return
	case tcell.KeyPgUp:
		u.events <- ActionEvent{Kind: PageUp}
		return
	case tcell.KeyPgDn:
		u.events <- ActionEvent{Kind: PageDown}
		return
	case tcell.KeyCtrlC:
		u.events <- ActionEvent{Kind: Quit}
		return
	}

	switch e.Rune() {
	case 'q':
		u.events <- ActionEvent{Kind: Quit}
	case 'b':
		u.events <- ActionEvent{Kind: Back}
	case 'f':
		u.events <- ActionEvent{Kind: Forward}
	case 'r':
		u.events <- ActionEvent{Kind: Reload}
	case 'g':
		u.events <- ActionEvent{Kind: URLBar}
	}
}

// Prompt enters modal input mode, showing label+initial text in the status
// bar. Enter emits InputEvent{Text}; Esc emits InputCancelEvent{}. While
// active, mouse clicks and navigation keys are ignored (Ctrl-C still quits).
func (u *UI) Prompt(label, initial string) {
	u.mu.Lock()
	u.inputMode = true
	u.label = label
	u.input = []rune(initial)
	u.mu.Unlock()
	u.events <- ResizeEvent{}
}

func (u *UI) handleInputKey(e *tcell.EventKey) {
	// Ctrl-C still quits, even while a prompt is active.
	if e.Key() == tcell.KeyCtrlC {
		u.events <- ActionEvent{Kind: Quit}
		return
	}

	var toSend Event

	switch e.Key() {
	case tcell.KeyEnter:
		u.mu.Lock()
		u.inputMode = false
		text := string(u.input)
		u.mu.Unlock()
		toSend = InputEvent{Text: text}
	case tcell.KeyEsc:
		u.mu.Lock()
		u.inputMode = false
		u.mu.Unlock()
		toSend = InputCancelEvent{}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		u.mu.Lock()
		if len(u.input) > 0 {
			u.input = u.input[:len(u.input)-1]
		}
		u.mu.Unlock()
		toSend = ResizeEvent{}
	default:
		if r := e.Rune(); r != 0 {
			u.mu.Lock()
			u.input = append(u.input, r)
			u.mu.Unlock()
			toSend = ResizeEvent{}
		}
	}

	if toSend != nil {
		u.events <- toSend
	}
}

// Draw renders the cell grid and the status bar, then flushes.
func (u *UI) Draw(view [][]cell.Cell, status string) {
	u.screen.Clear()
	for y, row := range view {
		for x, c := range row {
			if c.Continuation {
				continue // trailing column of a wide rune; tcell handles it
			}
			st := tcell.StyleDefault
			if c.HasFg {
				st = st.Foreground(tcell.NewRGBColor(int32(c.Fg.R), int32(c.Fg.G), int32(c.Fg.B)))
			}
			if c.HasBg {
				st = st.Background(tcell.NewRGBColor(int32(c.Bg.R), int32(c.Bg.G), int32(c.Bg.B)))
			}
			st = st.Bold(c.Bold).Italic(c.Italic).Underline(c.Underline).
				StrikeThrough(c.Strike).Reverse(c.Reverse).Dim(c.Dim)
			r := c.Rune
			if r == 0 {
				r = ' '
			}
			u.screen.SetContent(x, y, r, nil, st)
		}
	}
	u.drawStatus(status)
	u.screen.Show()
}

func (u *UI) drawStatus(status string) {
	w, h := u.screen.Size()
	y := h - 1
	u.mu.Lock()
	if u.inputMode {
		status = u.label + string(u.input) + "▏"
	}
	u.mu.Unlock()
	st := tcell.StyleDefault.Reverse(true)
	runes := []rune(status)
	for x := 0; x < w; x++ {
		r := ' '
		if x < len(runes) {
			r = runes[x]
		}
		u.screen.SetContent(x, y, r, nil, st)
	}
}
