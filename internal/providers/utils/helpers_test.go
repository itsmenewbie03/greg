package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"removes extra spaces", "hello    world", "hello world"},
		{"removes tabs", "hello\t\tworld", "hello world"},
		{"removes newlines", "hello\n\nworld", "hello world"},
		{"trims leading/trailing", "  hello world  ", "hello world"},
		{"handles mixed whitespace", "  hello  \t\n  world  ", "hello world"},
		{"handles empty string", "", ""},
		{"handles single space", " ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractYear(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"extracts 4-digit year", "Released in 2023", 2023},
		{"extracts from parentheses", "Movie (2020)", 2020},
		{"extracts 19xx year", "Classic 1999", 1999},
		{"returns first year found", "2019-2020", 2019},
		{"returns 0 for no year", "No year here", 0},
		{"returns 0 for 3-digit", "Year 999", 0},
		{"handles empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractYear(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"parses positive number", "123", 123},
		{"parses negative number", "-456", -456},
		{"parses with whitespace", "  789  ", 789},
		{"returns 0 for invalid", "abc", 0},
		{"returns 0 for empty", "", 0},
		{"returns 0 for decimal", "12.34", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"parses integer", "123", 123.0},
		{"parses decimal", "12.34", 12.34},
		{"parses with whitespace", "  45.67  ", 45.67},
		{"returns 0 for invalid", "abc", 0.0},
		{"returns 0 for empty", "", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFloat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"no truncation needed", "hello", 10, "hello"},
		{"truncates long string", "hello world", 8, "hello..."},
		{"handles exact length", "hello", 5, "hello"},
		{"truncates to 3 or less", "hello", 3, "hel"},
		{"truncates to 2", "hello", 2, "he"},
		{"handles empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitGenres(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"splits by comma", "Action, Comedy, Drama", []string{"Action", "Comedy", "Drama"}},
		{"splits by slash", "Action/Comedy/Drama", []string{"Action", "Comedy", "Drama"}},
		{"splits by pipe", "Action|Comedy|Drama", []string{"Action", "Comedy", "Drama"}},
		{"handles mixed delimiters", "Action, Comedy/Drama", []string{"Action", "Comedy", "Drama"}},
		{"removes empty entries", "Action,,Comedy", []string{"Action", "Comedy"}},
		{"trims whitespace", " Action , Comedy ", []string{"Action", "Comedy"}},
		{"handles empty string", "", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitGenres(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"Action", "Comedy", "Drama"}

	tests := []struct {
		name     string
		item     string
		expected bool
	}{
		{"finds exact match", "Action", true},
		{"finds case-insensitive", "action", true},
		{"finds uppercase", "ACTION", true},
		{"not found", "Horror", false},
		{"empty string not found", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{"removes duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"preserves order", []string{"c", "a", "b", "a"}, []string{"c", "a", "b"}},
		{"handles no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"handles empty slice", []string{}, []string{}},
		{"handles single item", []string{"a"}, []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveDuplicates(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"returns first non-empty", []string{"", "hello", "world"}, "hello"},
		{"returns first value", []string{"hello", "world"}, "hello"},
		{"handles all empty", []string{"", "", ""}, ""},
		{"handles no values", []string{}, ""},
		{"handles single value", []string{"hello"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultString(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultInt(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected int
	}{
		{"returns first non-zero", []int{0, 5, 10}, 5},
		{"returns first value", []int{5, 10}, 5},
		{"handles all zero", []int{0, 0, 0}, 0},
		{"handles no values", []int{}, 0},
		{"handles single value", []int{42}, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultInt(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"converts to lowercase", "Hello World", "hello world"},
		{"removes special chars", "Hello, World!", "hello world"},
		{"handles punctuation", "Re:Zero - Starting Life", "rezero starting life"},
		{"preserves alphanumeric", "Attack on Titan 2", "attack on titan 2"},
		{"cleans whitespace", "  Hello   World  ", "hello world"},
		{"handles empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
