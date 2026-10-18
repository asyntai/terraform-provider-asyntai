package provider

import (
	"encoding/json"
	"testing"
)

func TestToAPIValue(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		current interface{}
		want    string // JSON of the result
		wantErr bool
	}{
		{"bool true", "true", false, `true`, false},
		{"bool padded", " false ", true, `false`, false},
		{"bool bad", "maybe", false, ``, true},
		{"int", "42", float64(5), `42`, false},
		{"float", "0.7", float64(0), `0.7`, false},
		{"number bad", "many", float64(5), ``, true},
		{"list", `["en","de"]`, []interface{}{}, `["en","de"]`, false},
		{"list bad", `en,de`, []interface{}{}, ``, true},
		{"text", "Ava", "AI Assistant", `"Ava"`, false},
		{"null empty", "", nil, `null`, false},
		{"null text", "hello", nil, `"hello"`, false},
	}
	for _, c := range cases {
		got, err := toAPIValue("x", c.text, c.current, true)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error, got %v", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		buf, _ := json.Marshal(got)
		if string(buf) != c.want {
			t.Errorf("%s: got %s, want %s", c.name, buf, c.want)
		}
	}
}

func TestSameSetting(t *testing.T) {
	cases := []struct {
		configured string
		api        interface{}
		want       bool
	}{
		{"true", true, true},
		{"TRUE", true, true},
		{"false", true, false},
		{"12", float64(12), true},
		{"12.0", float64(12), true},
		{"13", float64(12), false},
		{`["en", "de"]`, []interface{}{"en", "de"}, true},
		{`["de","en"]`, []interface{}{"en", "de"}, false},
		{"Ava ", "Ava", true},
		{"Ava", "Max", false},
		{"", nil, true},
	}
	for _, c := range cases {
		if got := sameSetting(c.configured, c.api); got != c.want {
			t.Errorf("sameSetting(%q, %v) = %v, want %v", c.configured, c.api, got, c.want)
		}
	}
}

func TestFromAPIValue(t *testing.T) {
	cases := map[string]interface{}{
		"true":        true,
		"640":         float64(640),
		"0.25":        0.25,
		`["en","de"]`: []interface{}{"en", "de"},
		"":            nil,
		"#0f172a":     "#0f172a",
	}
	for want, in := range cases {
		if got := fromAPIValue(in); got != want {
			t.Errorf("fromAPIValue(%v) = %q, want %q", in, got, want)
		}
	}
}
