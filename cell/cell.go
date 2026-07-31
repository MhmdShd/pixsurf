// Package cell defines the terminal display cell shared by layout and ui.
package cell

// RGB is an 8-bit color.
type RGB struct{ R, G, B uint8 }

// Cell is one terminal character cell with optional colors and attributes.
// HasFg/HasBg false means "terminal default color".
type Cell struct {
	Rune                                           rune
	Fg, Bg                                         RGB
	HasFg, HasBg                                   bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool
}
