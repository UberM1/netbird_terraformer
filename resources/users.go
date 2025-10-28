package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// User represents a NetBird user
type User struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	AutoGroups    []string `json:"auto_groups"`
	Status        string   `json:"status"`
	Issued        string   `json:"issued"`
	LastLogin     string   `json:"last_login"`
	IsBlocked     bool     `json:"is_blocked"`
	IsServiceUser bool     `json:"is_service_user"`
	IsCurrent     bool     `json:"is_current"`
}

// Handler implements ResourceHandler for users
type UsersHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	groupMapping    map[string]string
}

// NewHandler creates a new users handler
func NewUsersHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *UsersHandler {
	return &UsersHandler{
		service:         service,
		terraformWriter: terraformWriter,
		groupMapping:    make(map[string]string),
	}
}

// SetGroupMapping sets the group ID to resource name mapping
func (h *UsersHandler) SetGroupMapping(groupMapping map[string]string) {
	h.groupMapping = groupMapping
}

// ImportAndGenerate imports users from NetBird and generates Terraform resources
func (h *UsersHandler) ImportAndGenerate() error {
	fmt.Printf("Importing users...\n")

	var users []User
	err := h.service.Get("/api/users", &users)
	if err != nil {
		return fmt.Errorf("failed to fetch users: %w", err)
	}

	for _, user := range users {
		// Skip users without email addresses, unless they are service users
		if user.Email == "" && !user.IsServiceUser {
			fmt.Printf("  Skipping user without email: ID=%s, Name=%s\n", user.ID, user.Name)
			continue
		}

		// Skip inactive service users without names
		if user.IsServiceUser && user.Email == "" && user.Name == "" {
			fmt.Printf("  Skipping unnamed service user: ID=%s\n", user.ID)
			continue
		}

		h.generateUserResource(user)
	}

	fmt.Printf("Imported %d users\n", len(users))
	return nil
}

// GetResourceMapping returns an empty mapping since users don't need to be referenced
func (h *UsersHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *UsersHandler) GetResourceType() string {
	return "user"
}

// generateUserResource generates a Terraform resource for a user
func (h *UsersHandler) generateUserResource(user User) {
	// Generate a unique resource name
	var resourceName string

	if resourceName == "" && user.Name != "" {
		resourceName = lib.SanitizeResourceName(user.Name)
	}

	// If still no valid name, use ID with role prefix
	if resourceName == "" {
		if user.Role == "admin" {
			resourceName = lib.SanitizeResourceName(fmt.Sprintf("admin_user_%s", user.ID))
		} else {
			resourceName = lib.SanitizeResourceName(fmt.Sprintf("user_%s", user.ID))
		}
	}

	// Build attributes according to the schema
	attributes := map[string]any{
		"id": user.ID,
	}

	// Required field for service users
	if user.IsServiceUser {
		attributes["is_service_user"] = true
	} else {
		attributes["is_service_user"] = false
	}

	if user.Email != "" {
		attributes["email"] = user.Email
	}

	if user.Name != "" {
		attributes["name"] = user.Name
	}

	if user.Role != "" {
		attributes["role"] = user.Role
	}

	if user.IsBlocked {
		attributes["is_blocked"] = true
	} else {
		attributes["is_blocked"] = false
	}

	// Convert auto_groups IDs to Terraform references
	if len(user.AutoGroups) > 0 {
		autoGroupRefs := make([]string, 0)
		for _, groupID := range user.AutoGroups {
			if groupResourceName, exists := h.groupMapping[groupID]; exists {
				terraformRef := lib.CreateTerraformReference("group", groupResourceName)
				autoGroupRefs = append(autoGroupRefs, terraformRef)
			} else {
				// Fallback to hardcoded ID if group not found in mapping
				autoGroupRefs = append(autoGroupRefs, groupID)
			}
		}
		attributes["auto_groups"] = autoGroupRefs
	}

	h.terraformWriter.AddResource("user", resourceName, attributes)
}
