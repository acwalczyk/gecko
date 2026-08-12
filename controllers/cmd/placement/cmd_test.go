package placement

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDomains(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"single domain", "example.com", []string{"example.com"}},
		{"multiple domains", "a.example.com,b.example.com", []string{"a.example.com", "b.example.com"}},
		{"trims whitespace", " a.example.com , b.example.com ", []string{"a.example.com", "b.example.com"}},
		{"drops empty entries from trailing comma", "a.example.com,", []string{"a.example.com"}},
		{"drops whitespace-only entries", "a.example.com, , ,b.example.com", []string{"a.example.com", "b.example.com"}},
		{"empty string → nil", "", nil},
		{"only commas → nil", ",,,", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, parseDomains(tc.input))
		})
	}
}
