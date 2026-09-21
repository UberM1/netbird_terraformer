package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Network represents a NetBird network
type Network struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// NetworkResource represents a resource exposed inside a network
type NetworkResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Address     string `json:"address"`
	Enabled     bool   `json:"enabled"`
	Groups      []any  `json:"groups"`
	// Type is derived from Address by NetBird and is not a Terraform argument.
	Type string `json:"type"`
}

// NetworkRouter represents a routing peer for a network
type NetworkRouter struct {
	ID         string   `json:"id"`
	Peer       string   `json:"peer"`
	PeerGroups []string `json:"peer_groups"`
	Metric     int      `json:"metric"`
	Masquerade bool     `json:"masquerade"`
	Enabled    bool     `json:"enabled"`
}

// NetworksHandler implements ResourceHandler for networks and their sub-resources
type NetworksHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	groupMapping    map[string]string
	peerResolver    *PeerResolver
	// network resource ID -> Terraform resource name, for policies that
	// reference a resource instead of a group.
	resourceNames map[string]string
}

// NetworkResourceName returns the Terraform resource name for a network
// resource ID, if that resource was generated.
func (h *NetworksHandler) NetworkResourceName(id string) (string, bool) {
	name, ok := h.resourceNames[id]
	return name, ok
}

// SetPeerResolver sets the resolver used to turn peer IDs into references
func (h *NetworksHandler) SetPeerResolver(resolver *PeerResolver) {
	h.peerResolver = resolver
}

// NewNetworksHandler creates a new networks handler
func NewNetworksHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *NetworksHandler {
	return &NetworksHandler{
		service:         service,
		terraformWriter: terraformWriter,
		groupMapping:    make(map[string]string),
		resourceNames:   make(map[string]string),
	}
}

// SetGroupMapping sets the group ID to resource name mapping
func (h *NetworksHandler) SetGroupMapping(groupMapping map[string]string) {
	h.groupMapping = groupMapping
}

// ImportAndGenerate imports networks, their resources and their routers
func (h *NetworksHandler) ImportAndGenerate() error {
	fmt.Printf("Importing networks...\n")

	var networks []Network
	err := h.service.Get("/api/networks", &networks)
	if err != nil {
		return fmt.Errorf("failed to fetch networks: %w", err)
	}

	resourceCount := 0
	routerCount := 0

	for _, network := range networks {
		networkResourceName := h.generateNetworkResource(network)

		var netResources []NetworkResource
		err := h.service.Get(fmt.Sprintf("/api/networks/%s/resources", network.ID), &netResources)
		if err != nil {
			fmt.Printf("  Warning: failed to fetch resources for network %s: %v\n", network.Name, err)
		} else {
			for _, netResource := range netResources {
				h.generateNetworkResourceResource(network, networkResourceName, netResource)
			}
			resourceCount += len(netResources)
		}

		var routers []NetworkRouter
		err = h.service.Get(fmt.Sprintf("/api/networks/%s/routers", network.ID), &routers)
		if err != nil {
			fmt.Printf("  Warning: failed to fetch routers for network %s: %v\n", network.Name, err)
			continue
		}
		for i, router := range routers {
			h.generateNetworkRouterResource(network, networkResourceName, router, i, len(routers))
		}
		routerCount += len(routers)
	}

	fmt.Printf("Imported %d networks, %d resources, %d routers\n", len(networks), resourceCount, routerCount)
	return nil
}

// GetResourceMapping returns an empty mapping since networks are referenced directly
func (h *NetworksHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *NetworksHandler) GetResourceType() string {
	return "network"
}

// groupRefs converts group IDs into Terraform references. The API returns
// groups either as bare IDs or as objects carrying an id field.
func (h *NetworksHandler) groupRefs(groups []any) []string {
	refs := make([]string, 0, len(groups))
	for _, group := range groups {
		var id string
		switch g := group.(type) {
		case string:
			id = g
		case map[string]any:
			if raw, ok := g["id"].(string); ok {
				id = raw
			}
		}
		if id == "" {
			continue
		}
		if resourceName, exists := h.groupMapping[id]; exists {
			refs = append(refs, lib.CreateTerraformReference("group", resourceName))
		}
	}
	return refs
}

// generateNetworkResource generates a Terraform resource for a network
func (h *NetworksHandler) generateNetworkResource(network Network) string {
	resourceName := lib.SanitizeResourceName(network.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("network_%s", network.ID)
	}

	// policies, resources and routers are read-only on this resource.
	attributes := map[string]any{
		"id":          network.ID,
		"name":        network.Name,
		"description": network.Description,
	}

	h.terraformWriter.AddResource("network", resourceName, attributes)
	return resourceName
}

// generateNetworkResourceResource generates a Terraform resource for a network resource
func (h *NetworksHandler) generateNetworkResourceResource(network Network, networkResourceName string, netResource NetworkResource) {
	resourceName := fmt.Sprintf("%s_%s", networkResourceName, lib.SanitizeResourceName(netResource.Name))

	attributes := map[string]any{
		// This resource imports as network_id/resource_id.
		"id":          fmt.Sprintf("%s/%s", network.ID, netResource.ID),
		"name":        netResource.Name,
		"description": netResource.Description,
		"address":     netResource.Address,
		"enabled":     netResource.Enabled,
		"network_id":  lib.RawValue(fmt.Sprintf("netbird_network.%s.id", networkResourceName)),
		"groups":      h.groupRefs(netResource.Groups),
	}

	h.resourceNames[netResource.ID] = resourceName
	h.terraformWriter.AddResource("network_resource", resourceName, attributes)
}

// generateNetworkRouterResource generates a Terraform resource for a network router
func (h *NetworksHandler) generateNetworkRouterResource(network Network, networkResourceName string, router NetworkRouter, index, total int) {
	// A network can have more than one routing peer, so only disambiguate when needed.
	resourceName := fmt.Sprintf("%s_router", networkResourceName)
	if total > 1 {
		resourceName = fmt.Sprintf("%s_router_%d", networkResourceName, index+1)
	}

	attributes := map[string]any{
		// This resource imports as network_id/router_id.
		"id":         fmt.Sprintf("%s/%s", network.ID, router.ID),
		"network_id": lib.RawValue(fmt.Sprintf("netbird_network.%s.id", networkResourceName)),
		"enabled":    router.Enabled,
		"masquerade": router.Masquerade,
		"metric":     router.Metric,
	}

	// peer and peer_groups are mutually exclusive.
	if len(router.PeerGroups) > 0 {
		refs := make([]any, 0, len(router.PeerGroups))
		for _, id := range router.PeerGroups {
			refs = append(refs, id)
		}
		attributes["peer_groups"] = h.groupRefs(refs)
	} else if router.Peer != "" {
		if h.peerResolver != nil {
			attributes["peer"] = h.peerResolver.Reference(router.Peer)
		} else {
			attributes["peer"] = router.Peer
		}
	}

	h.terraformWriter.AddResource("network_router", resourceName, attributes)
}
