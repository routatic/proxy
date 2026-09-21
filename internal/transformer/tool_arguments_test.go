package transformer

import (
	"encoding/json"
	"testing"
)

// Only key whitespace is repaired. Numbers and string values must survive the
// round-trip exactly as the provider sent them.
func TestNormalizeToolArgumentsPreservesValues(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trims whitespace around nested keys",
			in:   `{"questions":[{"options":[{"label":"A","description ":"answer"}]}]}`,
			want: `{"questions":[{"options":[{"label":"A","description":"answer"}]}]}`,
		},
		{
			name: "large integer keeps full precision",
			in:   `{"id":1234567890123456789}`,
			want: `{"id":1234567890123456789}`,
		},
		{
			name: "number formatting is not rewritten",
			in:   `{"price":1.0,"exp":1e21}`,
			want: `{"price":1.0,"exp":1e21}`,
		},
		{
			name: "html characters and ampersands are not escaped",
			in:   `{"html":"<b>bold</b>","amp":"a&b"}`,
			want: `{"html":"<b>bold</b>","amp":"a&b"}`,
		},
		{
			name: "invalid json is returned unchanged",
			in:   `{"description ":"unterminated}`,
			want: `{"description ":"unterminated}`,
		},
		{
			name: "trailing content is returned unchanged",
			in:   `{"a":1}{"b":2}`,
			want: `{"a":1}{"b":2}`,
		},
		{
			name: "surrogate escapes in values remain untouched",
			in:   `{"text ":"\ud800 \udc00 \uD83D\uDE00"}`,
			want: `{"text":"\ud800 \udc00 \uD83D\uDE00"}`,
		},
		{
			name: "string escape spellings remain untouched",
			in:   `{"text ":"\u0061\/\u2028"}`,
			want: `{"text":"\u0061\/\u2028"}`,
		},
		{
			name: "ambiguous padded keys preserve both values",
			in:   `{" description":"A","description ":"B"}`,
			want: `{" description":"A","description ":"B"}`,
		},
		{
			name: "nested collision preserves the entire argument",
			in:   `{"outer ":[{" child":{"a":1},"child ":{"b":2}}]}`,
			want: `{"outer ":[{" child":{"a":1},"child ":{"b":2}}]}`,
		},
		{
			name: "duplicate keys preserve both values",
			in:   `{"key":1,"key":2}`,
			want: `{"key":1,"key":2}`,
		},
		{
			name: "formatting and primitive values are preserved",
			in:   " \n{ \" key \" : [1.00, -0, 1E+21, true, false, null] }\t",
			want: " \n{ \"key\" : [1.00, -0, 1E+21, true, false, null] }\t",
		},
		{
			name: "escaped quotes and structural characters in strings",
			in:   `{" text ":"{\" key \": [\"quoted\\\"\", \"backslash\\\\\"]}"," next ":"ok"}`,
			want: `{"text":"{\" key \": [\"quoted\\\"\", \"backslash\\\\\"]}","next":"ok"}`,
		},
		{
			name: "escaped whitespace and quotes in keys",
			in:   `{"\u0020quote\"\t":"value"}`,
			want: `{"quote\"":"value"}`,
		},
		{
			name: "unicode whitespace in keys",
			in:   `{"\u00a0key\u2003":"value"}`,
			want: `{"key":"value"}`,
		},
		{
			name: "potentially lossy key remains unchanged",
			in:   `{" \ud800 ":"value"}`,
			want: `{" \ud800 ":"value"}`,
		},
		{
			name: "sibling objects have independent keys",
			in:   `[{" key ":"A"},{" key ":"B"}]`,
			want: `[{"key":"A"},{"key":"B"}]`,
		},
		{
			name: "empty trimmed key collision preserves input",
			in:   `{" ":1,"\t":2}`,
			want: `{" ":1,"\t":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeToolArguments(tt.in); got != tt.want {
				t.Fatalf("normalizeToolArguments(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// An exact key and a padded variant are ambiguous too. Preserve both values
// for the client to validate instead of choosing one on the client's behalf.
func TestNormalizeToolArgumentsPreservesExactKeyCollision(t *testing.T) {
	for _, raw := range []string{
		`{"description":"exact","description ":"padded"}`,
		`{"description ":"padded","description":"exact"}`,
		`{" description":"A","description ":"B"}`,
	} {
		for i := 0; i < 100; i++ {
			if got := normalizeToolArguments(raw); got != raw {
				t.Fatalf("normalizeToolArguments(%s) = %s, want unchanged input", raw, got)
			}
		}
	}
}

func FuzzNormalizeToolArguments(f *testing.F) {
	for _, raw := range []string{
		`{"text ":"\ud800","numbers":[1E+20,-0,1.00]}`,
		`{" description":"A","description ":"B"}`,
		`[{" key ":"{\"quoted\":\"value\"}"},{" key ":"\\"}]`,
		`{"\u0020key\t":{" child ":null}}`,
		`{" outer ":[true,false,null,[],{}]}`,
		`{"invalid":`,
		`"root string"`,
	} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got := normalizeToolArguments(raw)
		if !json.Valid([]byte(raw)) {
			if got != raw {
				t.Fatalf("invalid JSON changed: %q -> %q", raw, got)
			}
			return
		}
		if !json.Valid([]byte(got)) {
			t.Fatalf("normalization produced invalid JSON: %q -> %q", raw, got)
		}
		if again := normalizeToolArguments(got); again != got {
			t.Fatalf("normalization is not idempotent: %q -> %q -> %q", raw, got, again)
		}
	})
}
