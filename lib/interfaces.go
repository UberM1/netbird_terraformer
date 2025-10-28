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

// TerraformWriter handles writing Terraform files and managing imports
type TerraformWriter interface {
	AddResource(resourceType, name string, attributes map[string]interface{})
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
