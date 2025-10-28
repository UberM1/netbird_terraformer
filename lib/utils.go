package lib

import "strings"

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
