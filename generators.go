package main

import (
	"fmt"
	"os"
	"os/exec"
)

func TerraformInit(folderPath string) error {
	cmd := exec.Command("terraform", "init")
	cmd.Dir = folderPath

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func TerraformImport(folderPath string, resourceAddress string, resourceID string) error {
	cmd := exec.Command("terraform", "import", resourceAddress, resourceID)
	cmd.Dir = folderPath

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

type ResourceGenerator interface {
	InitResources() error
}

type GroupResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type Group struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Peers     []any           `json:"peers"`
	Resources []GroupResource `json:"resources,omitempty"` // Network resources
}

type GroupsGenerator struct {
	Service               *NetBirdService
	Generator             *TerraformGenerator
	groupIDToResourceName map[string]string
}

func (g *GroupsGenerator) InitResources() error {
	fmt.Printf("Importing groups...\n")

	var groups []Group
	err := g.Service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups: %w", err)
	}

	g.groupIDToResourceName = make(map[string]string)

	for _, group := range groups {
		resourceName := g.generateGroupResource(group)
		g.groupIDToResourceName[group.ID] = resourceName
	}

	fmt.Printf("Imported %d groups\n", len(groups))
	return nil
}

// GetGroupIDMapping returns the mapping from group IDs to resource names
func (g *GroupsGenerator) GetGroupIDMapping() map[string]string {
	return g.groupIDToResourceName
}

// generateGroupResource generates a Terraform resource
func (g *GroupsGenerator) generateGroupResource(group Group) string {
	resourceName := sanitizeResourceName(group.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("group_%s", group.ID)
	}

	// Extract peer IDs from the peers array
	peerIDs := make([]string, 0)
	if group.Peers != nil {
		for _, peer := range group.Peers {
			if peerMap, ok := peer.(map[string]interface{}); ok {
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

	if g.Generator != nil {
		g.Generator.AddResource("group", resourceName, attributes)
	}

	return resourceName
}

type Peer struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	IP                     string `json:"ip"`
	Connected              bool   `json:"connected"`
	LastSeen               string `json:"last_seen"`
	OS                     string `json:"os"`
	Version                string `json:"version"`
	Groups                 []any  `json:"groups"`
	SSHEnabled             bool   `json:"ssh_enabled"`
	LoginExpirationEnabled bool   `json:"login_expiration_enabled"`
}

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

type UsersGenerator struct {
	Service   *NetBirdService
	Generator *TerraformGenerator
}

// InitResources initializes user resources
func (u *UsersGenerator) InitResources() error {
	// This method is kept for compatibility but uses empty group mapping
	return u.InitResourcesWithGroupMapping(make(map[string]string))
}

// InitResourcesWithGroupMapping initializes user resources with group ID mapping
func (u *UsersGenerator) InitResourcesWithGroupMapping(groupIDToResourceName map[string]string) error {
	fmt.Printf("Importing users...\n")

	var users []User
	err := u.Service.Get("/api/users", &users)
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

		u.generateUserResource(user, groupIDToResourceName)
	}

	fmt.Printf("Imported %d users\n", len(users))
	return nil
}

// generateUserResource generates a Terraform resource for a user
func (u *UsersGenerator) generateUserResource(user User, groupIDToResourceName map[string]string) {
	// Generate a unique resource name
	var resourceName string

	if resourceName == "" && user.Name != "" {
		resourceName = sanitizeResourceName(user.Name)
	}

	// If still no valid name, use ID with role prefix
	if resourceName == "" {
		if user.Role == "admin" {
			resourceName = sanitizeResourceName(fmt.Sprintf("admin_user_%s", user.ID))
		} else {
			resourceName = sanitizeResourceName(fmt.Sprintf("user_%s", user.ID))
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
	}

	// Convert auto_groups IDs to Terraform references
	if len(user.AutoGroups) > 0 {
		autoGroupRefs := make([]string, 0)
		for _, groupID := range user.AutoGroups {
			if groupResourceName, exists := groupIDToResourceName[groupID]; exists {
				terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
				autoGroupRefs = append(autoGroupRefs, terraformRef)
			} else {
				// Fallback to hardcoded ID if group not found in mapping
				autoGroupRefs = append(autoGroupRefs, groupID)
			}
		}
		attributes["auto_groups"] = autoGroupRefs
	}

	if u.Generator != nil {
		u.Generator.AddResource("user", resourceName, attributes)
	}
}

type Policy struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Enabled             bool         `json:"enabled"`
	Rules               []PolicyRule `json:"rules"`
	SourcePostureChecks []string     `json:"source_posture_checks"`
}

type PolicyRule struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Description         string      `json:"description"`
	Enabled             bool        `json:"enabled"`
	Action              string      `json:"action"`
	Bidirectional       bool        `json:"bidirectional"`
	Protocol            string      `json:"protocol"`
	Ports               []string    `json:"ports"`
	PortRanges          []PortRange `json:"port_ranges"`
	Sources             []GroupInfo `json:"sources"`
	SourceResource      *Resource   `json:"sourceResource"`
	Destinations        []GroupInfo `json:"destinations"`
	DestinationResource *Resource   `json:"destinationResource"`
}

type PortRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type GroupInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PeersCount     int    `json:"peers_count"`
	ResourcesCount int    `json:"resources_count"`
	Issued         string `json:"issued"`
}

type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type PoliciesGenerator struct {
	Service   *NetBirdService
	Generator *TerraformGenerator
}

func (p *PoliciesGenerator) InitResources() error {
	// empty group mapping
	return p.InitResourcesWithGroupMapping(make(map[string]string))
}

func (p *PoliciesGenerator) InitResourcesWithGroupMapping(groupIDToResourceName map[string]string) error {
	fmt.Printf("Importing policies...\n")

	var policies []Policy
	err := p.Service.Get("/api/policies", &policies)
	if err != nil {
		return fmt.Errorf("failed to fetch policies: %w", err)
	}

	for _, policy := range policies {
		p.generatePolicyResource(policy, groupIDToResourceName)
	}

	fmt.Printf("Imported %d policies\n", len(policies))
	return nil
}

func (p *PoliciesGenerator) generatePolicyResource(policy Policy, groupIDToResourceName map[string]string) {
	resourceName := sanitizeResourceName(policy.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("policy_%s", policy.ID)
	}

	attributes := map[string]any{
		"id":          policy.ID,
		"name":        policy.Name,
		"description": policy.Description,
		"enabled":     policy.Enabled,
	}

	if len(policy.SourcePostureChecks) > 0 {
		attributes["source_posture_checks"] = policy.SourcePostureChecks
	}

	if len(policy.Rules) > 0 {
		rules := make([]interface{}, 0)
		for _, rule := range policy.Rules {
			ruleMap := map[string]any{
				"name":          rule.Name,
				"description":   rule.Description,
				"enabled":       rule.Enabled,
				"action":        rule.Action,
				"bidirectional": rule.Bidirectional,
				"protocol":      rule.Protocol,
			}

			// add ports
			if len(rule.Ports) > 0 {
				ruleMap["ports"] = rule.Ports
			}

			// add port ranges
			if len(rule.PortRanges) > 0 {
				portRanges := make([]map[string]any, 0)
				for _, portRange := range rule.PortRanges {
					portRanges = append(portRanges, map[string]any{
						"start": portRange.Start,
						"end":   portRange.End,
					})
				}
				ruleMap["port_ranges"] = portRanges
			}

			// Handle sources - use proper group mapping
			if len(rule.Sources) > 0 {
				sources := make([]string, 0)
				for _, source := range rule.Sources {
					if groupResourceName, exists := groupIDToResourceName[source.ID]; exists {
						terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
						sources = append(sources, terraformRef)
					} else {
						groupResourceName := sanitizeResourceName(source.Name)
						terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
						sources = append(sources, terraformRef)
					}
				}
				ruleMap["sources"] = sources
			}

			// Handle destinations - use proper group mapping
			if len(rule.Destinations) > 0 {
				destinations := make([]string, 0)
				for _, dest := range rule.Destinations {
					if groupResourceName, exists := groupIDToResourceName[dest.ID]; exists {
						terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
						destinations = append(destinations, terraformRef)
					} else {
						groupResourceName := sanitizeResourceName(dest.Name)
						terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
						destinations = append(destinations, terraformRef)
					}
				}
				ruleMap["destinations"] = destinations
			}

			if rule.SourceResource != nil {
				ruleMap["source_resource"] = map[string]any{
					"id":   rule.SourceResource.ID,
					"type": rule.SourceResource.Type,
				}
			}

			if rule.DestinationResource != nil {
				ruleMap["destination_resource"] = map[string]any{
					"id":   rule.DestinationResource.ID,
					"type": rule.DestinationResource.Type,
				}
			}

			rules = append(rules, ruleMap)
		}
		attributes["rules"] = rules
	}

	if p.Generator != nil {
		p.Generator.AddResource("policy", resourceName, attributes)
	}
}

type Route struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	NetworkID   string   `json:"network_id"`
	Network     string   `json:"network"`
	NetworkType string   `json:"network_type"`
	Peer        string   `json:"peer"`
	PeerGroups  []string `json:"peer_groups"`
	Metric      int      `json:"metric"`
	Masquerade  bool     `json:"masquerade"`
	Enabled     bool     `json:"enabled"`
	Groups      []string `json:"groups"`
	KeepRoute   bool     `json:"keep_route"`
}

type RoutesGenerator struct {
	Service   *NetBirdService
	Generator *TerraformGenerator
}

func (r *RoutesGenerator) InitResources() error {
	fmt.Printf("Importing routes...\n")

	var groups []Group
	err := r.Service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups for route mapping: %w", err)
	}

	groupIDToResourceName := make(map[string]string)
	for _, group := range groups {
		resourceName := sanitizeResourceName(group.Name)
		groupIDToResourceName[group.ID] = resourceName
	}

	var routes []Route
	err = r.Service.Get("/api/routes", &routes)
	if err != nil {
		return fmt.Errorf("failed to fetch routes: %w", err)
	}

	for _, route := range routes {
		r.generateRouteResource(route, groupIDToResourceName)
	}

	fmt.Printf("Imported %d routes\n", len(routes))
	return nil
}

func (r *RoutesGenerator) generateRouteResource(route Route, groupIDToResourceName map[string]string) {
	resourceName := sanitizeResourceName(route.NetworkID)
	if resourceName == "" {
		resourceName = sanitizeResourceName(route.Network)
	}
	if resourceName == "" {
		resourceName = fmt.Sprintf("route_%s", route.ID)
	}

	groupRefs := make([]string, 0)
	for _, groupID := range route.Groups {
		if resourceName, exists := groupIDToResourceName[groupID]; exists {
			terraformRef := fmt.Sprintf("netbird_group.%s.id", resourceName)
			groupRefs = append(groupRefs, terraformRef)
		}
	}

	peerGroupRefs := make([]string, 0)
	for _, groupID := range route.PeerGroups {
		if resourceName, exists := groupIDToResourceName[groupID]; exists {
			terraformRef := fmt.Sprintf("netbird_group.%s.id", resourceName)
			peerGroupRefs = append(peerGroupRefs, terraformRef)
		}
	}

	attributes := map[string]any{
		"id":          route.ID,
		"description": route.Description,
		"network_id":  route.NetworkID,
		"network":     route.Network,
		"peer":        route.Peer,
		"peer_groups": peerGroupRefs,
		"metric":      route.Metric,
		"masquerade":  route.Masquerade,
		"enabled":     route.Enabled,
		"groups":      groupRefs,
		"keep_route":  route.KeepRoute,
	}

	if r.Generator != nil {
		r.Generator.AddResource("route", resourceName, attributes)
	}
}
