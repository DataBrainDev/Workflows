package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func join(str1, str2 string) string {
	return strings.TrimSpace(str1 + " " + str2)
}

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

// New string functions
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

func concat(strs ...string) string {
	return strings.Join(strs, "")
}

func toUpperCase(s string) string {
	return strings.ToUpper(s)
}

func toLowerCase(s string) string {
	return strings.ToLower(s)
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func split(s, sep string) []string {
	return strings.Split(s, sep)
}

func replace(s, old, new string) string {
	return strings.Replace(s, old, new, 1) // Replace first occurrence only
}

func strLength(s string) int {
	return len(s)
}

func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

func includes(s, substr string) bool {
	return strings.Contains(s, substr)
}

func startsWith(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func endsWith(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// JSON functions
func parseJSON(s string) (interface{}, error) {
	var data interface{}
	err := json.Unmarshal([]byte(s), &data)
	return data, err
}

func stringifyJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func hasOwnProperty(obj map[string]interface{}, key string) bool {
	_, ok := obj[key]
	return ok
}

func keys(obj map[string]interface{}) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

func values(obj map[string]interface{}) []interface{} {
	vals := make([]interface{}, 0, len(obj))
	for _, v := range obj {
		vals = append(vals, v)
	}
	return vals
}

// Other functions
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func toFixed(num float64, digits int) string {
	return strconv.FormatFloat(num, 'f', digits, 64)
}
