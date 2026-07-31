package css

import (
	"strconv"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/style"
)

// parseColor extends style.ParseColor with the rgb()/rgba() functional
// forms (alpha ignored), accepting comma-, space- and slash-separated
// components and percentages.
func parseColor(v string) (cell.RGB, bool) {
	v = strings.TrimSpace(v)
	if c, ok := style.ParseColor(v); ok {
		return c, true
	}
	low := strings.ToLower(v)
	if !strings.HasPrefix(low, "rgb(") && !strings.HasPrefix(low, "rgba(") {
		return cell.RGB{}, false
	}
	open := strings.IndexByte(v, '(')
	end := strings.LastIndexByte(v, ')')
	if end < open {
		end = len(v) // tolerate a missing close paren
	}
	fields := strings.FieldsFunc(v[open+1:end], func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '/'
	})
	if len(fields) < 3 {
		return cell.RGB{}, false
	}
	var out [3]uint8
	for i := 0; i < 3; i++ {
		pct := strings.HasSuffix(fields[i], "%")
		f, err := strconv.ParseFloat(strings.TrimSuffix(fields[i], "%"), 64)
		if err != nil {
			return cell.RGB{}, false
		}
		if pct {
			f = f * 255 / 100
		}
		if f < 0 {
			f = 0
		}
		if f > 255 {
			f = 255
		}
		out[i] = uint8(f + 0.5)
	}
	return cell.RGB{R: out[0], G: out[1], B: out[2]}, true
}
