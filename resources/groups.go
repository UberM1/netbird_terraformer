package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Group represents a NetBird group
type Group struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Peers     []any           `json:"peers"`
	Resources []GroupResource `json:"resources,omitempty"`
}

// GroupResource represents a resource within a group
type GroupResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// GroupsHandler implements ResourceHandler for groups
type GroupsHandler struct {
	service          lib.NetBirdAPI
	terraformWriter  lib.TerraformWriter
	idToResourceName map[string]string
}

// NewGroupsHandler creates a new groups handler
func NewGroupsHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *GroupsHandler {
	return &GroupsHandler{
		service:          service,
		terraformWriter:  terraformWriter,
		idToResourceName: make(map[string]string),
	}
}

// ImportAndGenerate imports groups from NetBird and generates Terraform resources
func (h *GroupsHandler) ImportAndGenerate() error {
	fmt.Printf("Importing groups...\n")

	var groups []Group
	err := h.service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups: %w", err)
	}

	for _, group := range groups {
		resourceName := h.generateGroupResource(group)
		h.idToResourceName[group.ID] = resourceName
	}

	fmt.Printf("Imported %d groups\n", len(groups))
	return nil
}

// GetResourceMapping returns the mapping from group IDs to resource names
func (h *GroupsHandler) GetResourceMapping() map[string]string {
	return h.idToResourceName
}

// GetResourceType returns the resource type
func (h *GroupsHandler) GetResourceType() string {
	return "group"
}

// generateGroupResource generates a Terraform resource for a group
func (h *GroupsHandler) generateGroupResource(group Group) string {
	resourceName := lib.SanitizeResourceName(group.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("group_%s", group.ID)
	}

	// Extract peer IDs from the peers array
	peerIDs := make([]string, 0)
	if group.Peers != nil {
		for _, peer := range group.Peers {
			if peerMap, ok := peer.(map[string]any); ok {
				if id, exists := peerMap["id"]; exists {
					if idStr, ok := id.(string); ok && idStr != "" {
						peerIDs = append(peerIDs, idStr)
					}
				}
			}
		}
	}

	attributes := map[string]any{
		"id":   group.ID,
		"name": group.Name,
	}

	if len(group.Resources) > 0 {
		resourceIDs := make([]string, 0)
		for _, resource := range group.Resources {
			if resource.ID != "" {
				resourceIDs = append(resourceIDs, resource.ID)
			}
		}
		if len(resourceIDs) > 0 {
			attributes["resources"] = resourceIDs
		}
	}

	h.terraformWriter.AddResource("group", resourceName, attributes)
	return resourceName
}
