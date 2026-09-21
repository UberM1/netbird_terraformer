package lib

import (
	"sort"
	"strings"
	"unicode"
)

// SanitizeResourceName sanitizes a string to be used as a Terraform resource name
func SanitizeResourceName(input string) string {
	name := strings.ReplaceAll(input, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "@", "_")

	name = strings.ReplaceAll(name, "(", "")
	name = strings.ReplaceAll(name, ")", "")
	name = strings.ReplaceAll(name, ",", "")
	name = strings.ReplaceAll(name, ":", "")

	name = strings.ToLower(name)

	// Terraform names allow letters, digits, underscores and dashes only.
	// Unicode letters are valid, so accented names are preserved as-is.
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)

	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}

	name = strings.Trim(name, "_")

	if len(name) > 0 && (name[0] >= '0' && name[0] <= '9') {
		name = "resource_" + name
	}

	if name == "" {
		name = "unnamed_resource"
	}

	return name
}

// EscapeString escapes special characters in strings for Terraform
func EscapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// GetBlockName converts plural list names to singular block names
func GetBlockName(key string) string {
	switch key {
	case "rules":
		return "rule"
	case "port_ranges":
		return "port_range"
	case "sources":
		return "source"
	case "destinations":
		return "destination"
	default:
		return key
	}
}

// CreateTerraformReference creates a Terraform reference string
func CreateTerraformReference(resourceType, resourceName string) string {
	return "netbird_" + resourceType + "." + resourceName + ".id"
}

// UniqueResourceNames maps record IDs to Terraform resource names. NetBird
// allows two records to share a name (auto-created temporary access policies do
// this), but Terraform requires unique resource names. Colliding names get a
// short ID suffix, which stays stable no matter what order the API returns.
func UniqueResourceNames(idToName map[string]string) map[string]string {
	counts := make(map[string]int, len(idToName))
	for _, name := range idToName {
		counts[name]++
	}

	result := make(map[string]string, len(idToName))
	for id, name := range idToName {
		if counts[name] > 1 {
			result[id] = name + "_" + ShortID(id)
		} else {
			result[id] = name
		}
	}
	return result
}

// ShortID returns a stable, name-safe suffix derived from a NetBird ID
func ShortID(id string) string {
	const suffixLen = 6
	if len(id) > suffixLen {
		id = id[len(id)-suffixLen:]
	}
	return SanitizeResourceName(id)
}

// Make key generation deterministic
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
