package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Route represents a NetBird route
type Route struct {
	ID            string   `json:"id"`
	Description   string   `json:"description"`
	NetworkID     string   `json:"network_id"`
	Network       string   `json:"network"`
	NetworkType   string   `json:"network_type"`
	Peer          string   `json:"peer"`
	PeerGroups    []string `json:"peer_groups"`
	Metric        int      `json:"metric"`
	Masquerade    bool     `json:"masquerade"`
	Enabled       bool     `json:"enabled"`
	Groups        []string `json:"groups"`
	KeepRoute     bool     `json:"keep_route"`
	Domains       []string `json:"domains"`
	SkipAutoApply bool     `json:"skip_auto_apply"`
}

// RouteGroup represents a NetBird group (minimal struct for group fetching)
type RouteGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Handler implements ResourceHandler for routes
type RoutesHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	peerResolver    *PeerResolver
}

// SetPeerResolver sets the resolver used to turn peer IDs into references
func (h *RoutesHandler) SetPeerResolver(resolver *PeerResolver) {
	h.peerResolver = resolver
}

// NewHandler creates a new routes handler
func NewRoutesHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *RoutesHandler {
	return &RoutesHandler{
		service:         service,
		terraformWriter: terraformWriter,
	}
}

// ImportAndGenerate imports routes from NetBird and generates Terraform resources
func (h *RoutesHandler) ImportAndGenerate() error {
	fmt.Printf("Importing routes...\n")

	// Fetch groups for group mapping
	var groups []RouteGroup
	err := h.service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups for route mapping: %w", err)
	}

	groupIDToResourceName := make(map[string]string)
	for _, group := range groups {
		resourceName := lib.SanitizeResourceName(group.Name)
		groupIDToResourceName[group.ID] = resourceName
	}

	// Fetch routes
	var routes []Route
	err = h.service.Get("/api/routes", &routes)
	if err != nil {
		return fmt.Errorf("failed to fetch routes: %w", err)
	}

	for _, route := range routes {
		h.generateRouteResource(route, groupIDToResourceName)
	}

	fmt.Printf("Imported %d routes\n", len(routes))
	return nil
}

// GetResourceMapping returns an empty mapping since routes don't need to be referenced
func (h *RoutesHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *RoutesHandler) GetResourceType() string {
	return "route"
}

// resolvePeer turns a peer ID into a data source reference when possible
func (h *RoutesHandler) resolvePeer(peerID string) any {
	if h.peerResolver == nil {
		return peerID
	}
	return h.peerResolver.Reference(peerID)
}

// generateRouteResource generates a Terraform resource for a route
func (h *RoutesHandler) generateRouteResource(route Route, groupIDToResourceName map[string]string) {
	resourceName := lib.SanitizeResourceName(route.NetworkID)
	if resourceName == "" {
		resourceName = lib.SanitizeResourceName(route.Network)
	}
	if resourceName == "" {
		resourceName = fmt.Sprintf("route_%s", route.ID)
	}

	groupRefs := make([]string, 0)
	for _, groupID := range route.Groups {
		if resourceName, exists := groupIDToResourceName[groupID]; exists {
			terraformRef := lib.CreateTerraformReference("group", resourceName)
			groupRefs = append(groupRefs, terraformRef)
		}
	}

	peerGroupRefs := make([]string, 0)
	for _, groupID := range route.PeerGroups {
		if resourceName, exists := groupIDToResourceName[groupID]; exists {
			terraformRef := lib.CreateTerraformReference("group", resourceName)
			peerGroupRefs = append(peerGroupRefs, terraformRef)
		}
	}

	attributes := map[string]any{
		"id":          route.ID,
		"description": route.Description,
		"network_id":  route.NetworkID,
		"network":     route.Network,
		"peer":        h.resolvePeer(route.Peer),
		"peer_groups": peerGroupRefs,
		"metric":      route.Metric,
		"masquerade":  route.Masquerade,
		"enabled":     route.Enabled,
		"groups":      groupRefs,
		"keep_route":  route.KeepRoute,
	}

	// A route targets either a network prefix or a domain list.
	if len(route.Domains) > 0 {
		attributes["domains"] = route.Domains
	}
	if route.SkipAutoApply {
		attributes["skip_auto_apply"] = route.SkipAutoApply
	}

	h.terraformWriter.AddResource("route", resourceName, attributes)
}
