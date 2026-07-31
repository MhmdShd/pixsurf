package css

import (
	"strconv"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/style"
)

// parseColor extends style.ParseColor with the rgb()/rgba() functional
// forms (alpha ignored), accepting comma-, space- and slash-separated
// components and percentages. var() references resolve to their
// fallback value first (see resolveVar).
func parseColor(v string) (cell.RGB, bool) {
	v = strings.TrimSpace(resolveVar(v))
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

// resolveVar replaces each var(--name, fallback) reference in v with its
// fallback value. Custom property definitions are not tracked — sites
// that matter (e.g. Wikipedia's body{background-color:var(--x,#f8f9fa)})
// ship literal fallbacks for non-supporting engines, and those are the
// values a light-theme render wants. A var() without a fallback resolves
// to nothing, leaving the value unparseable as before. Nested fallbacks
// resolve up to a small fixed depth.
func resolveVar(v string) string {
	for iter := 0; iter < 4; iter++ {
		i := strings.Index(strings.ToLower(v), "var(")
		if i < 0 {
			return v
		}
		depth, end, comma := 0, -1, -1
		for k := i + 3; k < len(v) && end < 0; k++ {
			switch v[k] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = k
				}
			case ',':
				if depth == 1 && comma < 0 {
					comma = k
				}
			}
		}
		if end < 0 {
			return v
		}
		fb := ""
		if comma >= 0 {
			fb = strings.TrimSpace(v[comma+1 : end])
		}
		v = v[:i] + fb + v[end+1:]
	}
	return v
}
