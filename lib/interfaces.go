package lib

import "os"

// ResourceHandler defines the interface for resource-specific handlers
type ResourceHandler interface {
	// ImportAndGenerate imports resources from NetBird and generates Terraform files
	ImportAndGenerate() error

	// GetResourceMapping returns mapping of resource IDs to Terraform resource names
	GetResourceMapping() map[string]string

	// GetResourceType returns the resource type (e.g., "group", "user", "policy")
	GetResourceType() string
}

// TerraformResource represents a Terraform resource or data source
type TerraformResource struct {
	Type       string
	Name       string
	Attributes map[string]interface{}
	IsData     bool   // true for data sources, false for resources
	ID         string // stored separately for import, not written to .tf files
}

// ImportCommand represents a terraform import command to be executed
type ImportCommand struct {
	ResourceAddress string
	ResourceID      string
}

// ObjectList marks a list of objects that must be written as a list attribute
// (foo = [{ ... }]) rather than as repeated blocks (foo { ... }). The NetBird
// provider uses both shapes: policy "rule" is a block list, while nameserver
// group "nameservers" is an attributes list.
type ObjectList []map[string]interface{}

// ObjectValue marks a single object written as an attribute (foo = { ... })
// rather than as a block (foo { ... }). Policy source_resource and
// destination_resource use this shape.
type ObjectValue map[string]interface{}

// RawValue is written verbatim, without quoting. Needed for required fields
// whose empty value must still be emitted, such as "[]".
type RawValue string

// ForceString is a quoted string that is written even when empty.
//
// Plain strings are dropped when empty, which is correct for attributes the API
// omits entirely. It is wrong for attributes the API returns as "": the config
// then has no value, Terraform reads that as null, and every plan reports a
// no-op update of "" -> null. Use this for those.
type ForceString string

// TerraformWriter handles writing Terraform files and managing imports
type TerraformWriter interface {
	AddResource(resourceType, name string, attributes map[string]interface{})
	AddResourceNoImport(resourceType, name string, attributes map[string]interface{})
	AddDataSource(dataType, name string, attributes map[string]interface{})
	WriteResource(file *os.File, resource TerraformResource) error
	QueueImport(resourceType, name string, resourceID string)
	GetImportCommands() []ImportCommand
}

// NetBirdAPI defines the interface for NetBird API operations
type NetBirdAPI interface {
	Get(endpoint string, result interface{}) error
}

// Config represents the application configuration
type Config struct {
	ServerURL  string
	APIToken   string
	Debug      bool
	AutoImport bool
}
