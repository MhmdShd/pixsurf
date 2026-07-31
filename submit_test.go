package main

import (
	"net/url"
	"testing"

	"github.com/MhmdShd/pixsurf/layout"
)

func TestSubmitURLBasic(t *testing.T) {
	f := layout.Form{
		Action: "https://s.example/search",
		Method: "get",
		Hidden: url.Values{"lang": {"en"}},
	}
	fields := []layout.Field{{Name: "q", FormIdx: 0}}
	values := layout.FormValues{layout.ValuesKey(f.Action, "q"): "cats"}

	got, err := submitURL(f, fields, values)
	if err != nil {
		t.Fatalf("submitURL: %v", err)
	}
	want := "https://s.example/search?lang=en&q=cats"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubmitURLReplacesQuery(t *testing.T) {
	f := layout.Form{
		Action: "https://s.example/search?old=1",
		Method: "get",
		Hidden: url.Values{},
	}
	fields := []layout.Field{{Name: "q", Value: "dogs", FormIdx: 0}}

	got, err := submitURL(f, fields, nil)
	if err != nil {
		t.Fatalf("submitURL: %v", err)
	}
	want := "https://s.example/search?q=dogs"
	if got != want {
		t.Errorf("got %q, want %q (old query must be dropped)", got, want)
	}
}

func TestSubmitURLFieldOverridesHidden(t *testing.T) {
	f := layout.Form{
		Action: "https://s.example/search",
		Method: "get",
		Hidden: url.Values{"q": {"hiddenval"}},
	}
	fields := []layout.Field{{Name: "q", Value: "typed", FormIdx: 0}}

	got, err := submitURL(f, fields, nil)
	if err != nil {
		t.Fatalf("submitURL: %v", err)
	}
	want := "https://s.example/search?q=typed"
	if got != want {
		t.Errorf("got %q, want %q (field must win over hidden)", got, want)
	}
}

func TestSubmitURLErrors(t *testing.T) {
	post := layout.Form{Action: "https://s.example/search", Method: "post", Hidden: url.Values{}}
	if _, err := submitURL(post, nil, nil); err == nil {
		t.Error("POST form: want error, got nil")
	}

	garbage := layout.Form{Action: ":not a url", Method: "get", Hidden: url.Values{}}
	if _, err := submitURL(garbage, nil, nil); err == nil {
		t.Error("garbage action: want error, got nil")
	}
}

func TestSubmitURLNameHandling(t *testing.T) {
	f := layout.Form{
		Action: "https://s.example/search",
		Method: "get",
		Hidden: url.Values{},
	}
	fields := []layout.Field{
		{Name: "", Value: "ignored", FormIdx: 0}, // nameless: skipped
		{Name: "q", Value: "", FormIdx: 0},       // named, empty value: included
	}

	got, err := submitURL(f, fields, nil)
	if err != nil {
		t.Fatalf("submitURL: %v", err)
	}
	want := "https://s.example/search?q="
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
