package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// countryToAlpha3 converts a country name to its ISO Alpha-3 code.
// Returns "   " if the country is not found in the map.
func countryToAlpha3(country string) string {
	countryMap := map[string]string{
		"united states":  "USA",
		"united kingdom": "GBR",
		"germany":        "DEU",
		"france":         "FRA",
		"canada":         "CAN",
		"australia":      "AUS",
		"india":          "IND",
		"china":          "CHN",
		"japan":          "JPN",
		"brazil":         "BRA",
		"israel":         "IL",
	}

	normalized := strings.ToLower(strings.TrimSpace(country))
	if code, ok := countryMap[normalized]; ok {
		return code
	}
	return "   "
}

// truncate shortens a string to the specified maximum length.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// join concatenates two strings with a space in between.
func join(str1, str2 string) string {
	return strings.TrimSpace(str1 + " " + str2)
}

// toNumber converts various types (float64, int, string) to a float64.
// Returns an error if the type is unsupported or conversion fails.
func toNumber(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		if v == "" {
			return 0, nil
		}
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported type for toNumber: %T", value)
	}
}

// toBoolean converts various types (bool, string) to a boolean.
// Returns an error if the type is unsupported or conversion fails.
func toBoolean(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		if v == "" {
			return false, nil
		}
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("unsupported type for toBoolean: %T", value)
	}
}

// substring extracts a portion of a string between start and end indices.
func substring(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start > end {
		start, end = end, start
	}
	if start > len(s) || end < 0 {
		return ""
	}
	return s[start:end]
}

// concat concatenates multiple strings into one.
func concat(strs ...string) string {
	return strings.Join(strs, "")
}

// toUpperCase converts a string to uppercase.
func toUpperCase(s string) string {
	return strings.ToUpper(s)
}

// toLowerCase converts a string to lowercase.
func toLowerCase(s string) string {
	return strings.ToLower(s)
}

// trim removes leading and trailing whitespace from a string.
func trim(s string) string {
	return strings.TrimSpace(s)
}

// split splits a string into a slice of substrings based on a separator.
func split(s, sep string) []string {
	return strings.Split(s, sep)
}

// replace replaces the first occurrence of a substring with a new string.
func replace(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}

// strLength returns the length of a string.
func strLength(s string) int {
	return len(s)
}

// indexOf returns the index of the first occurrence of a substring in a string.
// Returns -1 if the substring is not found.
func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

// includes checks if a substring exists within a string.
func includes(s, substr string) bool {
	return strings.Contains(s, substr)
}

// startsWith checks if a string starts with a specified prefix.
func startsWith(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// endsWith checks if a string ends with a specified suffix.
func endsWith(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// parseJSON parses a JSON string into a Go data structure (map, slice, etc.).
func parseJSON(s string) (interface{}, error) {
	var data interface{}
	err := json.Unmarshal([]byte(s), &data)
	return data, err
}

// stringifyJSON converts a Go data structure into a JSON string.
func stringifyJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// hasOwnProperty checks if a map contains a specific key.
func hasOwnProperty(obj map[string]interface{}, key string) bool {
	_, ok := obj[key]
	return ok
}

// keys returns a slice of all keys in a map.
func keys(obj map[string]interface{}) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

// values returns a slice of all values in a map.
func values(obj map[string]interface{}) []interface{} {
	vals := make([]interface{}, 0, len(obj))
	for _, v := range obj {
		vals = append(vals, v)
	}
	return vals
}

// parseInt converts a string to an integer.
// Returns 0 if the string is empty.
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// parseFloat converts a string to a float64.
// Returns 0 if the string is empty.
func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

// toFixed formats a float64 to a string with a specified number of decimal places.
func toFixed(num float64, digits int) string {
	return strconv.FormatFloat(num, 'f', digits, 64)
}
