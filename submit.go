package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/MhmdShd/pixsurf/layout"
)

// submitURL builds the GET submission URL for form f.
//
// Contract: the caller passes ONLY the fields belonging to f (pre-filtered
// by FormIdx). Each field's value is values[layout.ValuesKey(f.Action, name)]
// when present, else Field.Value. Fields with an empty Name are skipped.
// Field values are merged over f.Hidden (field wins on collision), and the
// action URL's existing query is replaced by the encoded result. Returns an
// error for a non-"get" method or an unparseable action.
func submitURL(f layout.Form, fields []layout.Field, values layout.FormValues) (string, error) {
	if f.Method != "get" {
		return "", fmt.Errorf("unsupported form (%s)", strings.ToUpper(f.Method))
	}
	u, err := url.Parse(f.Action)
	if err != nil {
		return "", fmt.Errorf("bad form action: %v", err)
	}
	q := url.Values{}
	for name, vals := range f.Hidden {
		q[name] = append([]string(nil), vals...)
	}
	for _, fl := range fields {
		if fl.Name == "" {
			continue
		}
		val := fl.Value
		if v, ok := values[layout.ValuesKey(f.Action, fl.Name)]; ok {
			val = v
		}
		q.Set(fl.Name, val)
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	return u.String(), nil
}
