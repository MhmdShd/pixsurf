// Package cell defines the terminal display cell shared by layout and ui.
package cell

// RGB is an 8-bit color.
type RGB struct{ R, G, B uint8 }

// Cell is one terminal character cell with optional colors and attributes.
// HasFg/HasBg false means "terminal default color".
// Continuation marks the trailing column of a wide (2-column) rune: the
// preceding cell holds the rune, this cell holds none and is never drawn.
type Cell struct {
	Rune                                          rune
	Fg, Bg                                        RGB
	HasFg, HasBg                                  bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool
	Continuation                                  bool
}
