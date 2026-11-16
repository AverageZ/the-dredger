package urlutil

import "testing"

const exampleDomain = "example.com"

func TestDomain(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain host", "https://example.com/a/b", exampleDomain},
		{"strips www", "https://www.example.com/", exampleDomain},
		{"strips port", "http://example.com:8080/x", exampleDomain},
		{"lowercases", "https://EXAMPLE.COM/X", exampleDomain},
		{"trims space", "  https://example.com  ", exampleDomain},
		{"subdomain kept", "https://blog.example.com", "blog.example.com"},
		{"no host", "not a url", ""},
		{"mailto", "mailto:a@example.com", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Domain(tt.raw); got != tt.want {
				t.Errorf("Domain(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"slug separators", "https://example.com/blog/wave-function-collapse", " blog wave function collapse"},
		{"underscores", "https://example.com/a_b", " a b"},
		{"query included", "https://example.com/p?q=shader", " p q shader"},
		{"encoded query", "https://example.com/?q=wave%20function", "  q wave function"},
		{"plus query", "https://example.com/?q=wave+function", "  q wave function"},
		{"malformed query", "https://example.com/?q=%xx", "  q %xx"},
		{"root", "https://example.com/", " "},
		{"no path", "https://example.com", ""},
		{"unparseable", "://bad", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Path(tt.raw); got != tt.want {
				t.Errorf("Path(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
