package main

import (
	"fmt"
	"os"
	"os/exec"
)

func TerraformInit(folderPath string) error {
	cmd := exec.Command("terraform", "init")
	cmd.Dir = folderPath

	// Optional: pipe output to see it in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func TerraformImport(folderPath string, resourceAddress string, resourceID string) error {
	cmd := exec.Command("terraform", "import", resourceAddress, resourceID)
	cmd.Dir = folderPath

	// Optional: pipe output to see it in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// ResourceGenerator interface for all resource generators
type ResourceGenerator interface {
	InitResources() error
}

// GroupResource represents a network resource in a group
type GroupResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Group represents a NetBird group
type Group struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Peers     []any           `json:"peers"`
	Resources []GroupResource `json:"resources,omitempty"` // Network resources
}

// GroupsGenerator generates Terraform configuration for NetBird groups
type GroupsGenerator struct {
	Service               *NetBirdService
	Generator             *TerraformGenerator
	groupIDToResourceName map[string]string
}

// InitResources initializes group resources
func (g *GroupsGenerator) InitResources() error {
	fmt.Printf("Importing groups...\n")

	var groups []Group
	err := g.Service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups: %w", err)
	}

	// Initialize the mapping
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

// generateGroupResource generates a Terraform resource for a group and returns the resource name
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

	// Create the attributes map with only the required and optional fields
	attributes := map[string]any{
		"id":   group.ID,
		"name": group.Name,
	}

	// Skip peers field - it's managed automatically by NetBird
	// Peers are assigned to groups automatically when they connect

	// Extract resource IDs if there are any (this is an optional field)
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

// Peer represents a NetBird peer (used for data source generation)
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

// UsersGenerator generates Terraform configuration for NetBird users
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

	// Optional fields - only include if not empty/default
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

// Policy represents a NetBird policy
type Policy struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Enabled             bool         `json:"enabled"`
	Rules               []PolicyRule `json:"rules"`
	SourcePostureChecks []string     `json:"source_posture_checks"`
}

// PolicyRule represents a policy rule
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

// PortRange represents a port range
type PortRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// GroupInfo represents group information
type GroupInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PeersCount     int    `json:"peers_count"`
	ResourcesCount int    `json:"resources_count"`
	Issued         string `json:"issued"`
}

// Resource represents a resource
type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// PoliciesGenerator generates Terraform configuration for NetBird policies
type PoliciesGenerator struct {
	Service   *NetBirdService
	Generator *TerraformGenerator
}

// InitResources initializes policy resources
func (p *PoliciesGenerator) InitResources() error {
	// This method is kept for compatibility but uses empty group mapping
	return p.InitResourcesWithGroupMapping(make(map[string]string))
}

// InitResourcesWithGroupMapping initializes policy resources with group ID mapping
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

// generatePolicyResource generates a Terraform resource for a policy
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

	// Only include source_posture_checks if not empty
	if len(policy.SourcePostureChecks) > 0 {
		attributes["source_posture_checks"] = policy.SourcePostureChecks
	}

	// Convert rules to detailed Terraform format
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

			// Handle ports
			if len(rule.Ports) > 0 {
				ruleMap["ports"] = rule.Ports
			}

			// Handle port ranges
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
						// Fallback to sanitizing the name if not in mapping
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
						// Fallback to sanitizing the name if not in mapping
						groupResourceName := sanitizeResourceName(dest.Name)
						terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
						destinations = append(destinations, terraformRef)
					}
				}
				ruleMap["destinations"] = destinations
			}

			// Handle source resource
			if rule.SourceResource != nil {
				ruleMap["source_resource"] = map[string]any{
					"id":   rule.SourceResource.ID,
					"type": rule.SourceResource.Type,
				}
			}

			// Handle destination resource
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

// Route represents a NetBird route
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

// RoutesGenerator generates Terraform configuration for NetBird routes
type RoutesGenerator struct {
	Service   *NetBirdService
	Generator *TerraformGenerator
}

// InitResources initializes route resources
func (r *RoutesGenerator) InitResources() error {
	fmt.Printf("Importing routes...\n")

	// First fetch groups for ID-to-name mapping
	var groups []Group
	err := r.Service.Get("/api/groups", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch groups for route mapping: %w", err)
	}

	// Create mapping from group ID to sanitized resource name
	groupIDToResourceName := make(map[string]string)
	for _, group := range groups {
		resourceName := sanitizeResourceName(group.Name)
		groupIDToResourceName[group.ID] = resourceName
	}

	// Now fetch routes
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

// generateRouteResource generates a Terraform resource for a route
func (r *RoutesGenerator) generateRouteResource(route Route, groupIDToResourceName map[string]string) {
	resourceName := sanitizeResourceName(route.NetworkID)
	if resourceName == "" {
		resourceName = sanitizeResourceName(route.Network)
	}
	if resourceName == "" {
		resourceName = fmt.Sprintf("route_%s", route.ID)
	}

	// Convert group IDs to Terraform references
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

// // SetupKey represents a NetBird setup key
// type SetupKey struct {
// 	ID         string   `json:"id"`
// 	Name       string   `json:"name"`
// 	Type       string   `json:"type"`
// 	CreatedAt  string   `json:"created_at"`
// 	ExpiresAt  string   `json:"expires_at"`
// 	Revoked    bool     `json:"revoked"`
// 	UsedTimes  int      `json:"used_times"`
// 	LastUsed   string   `json:"last_used"`
// 	State      string   `json:"state"`
// 	AutoGroups []string `json:"auto_groups"`
// 	UsageLimit int      `json:"usage_limit"`
// 	Ephemeral  bool     `json:"ephemeral"`
// }

// // SetupKeysGenerator generates Terraform configuration for NetBird setup keys
// type SetupKeysGenerator struct {
// 	Service   *NetBirdService
// 	Generator *TerraformGenerator
// }

// // InitResources initializes setup key resources
// func (s *SetupKeysGenerator) InitResources() error {
// 	// This method is kept for compatibility but uses empty group mapping
// 	return s.InitResourcesWithGroupMapping(make(map[string]string))
// }

// // InitResourcesWithGroupMapping initializes setup key resources with group ID mapping
// func (s *SetupKeysGenerator) InitResourcesWithGroupMapping(groupIDToResourceName map[string]string) error {
// 	fmt.Printf("Importing setup keys...\n")

// 	var setupKeys []SetupKey
// 	err := s.Service.Get("/api/setup-keys", &setupKeys)
// 	if err != nil {
// 		return fmt.Errorf("failed to fetch setup keys: %w", err)
// 	}

// 	for _, setupKey := range setupKeys {
// 		s.generateSetupKeyResource(setupKey, groupIDToResourceName)
// 	}

// 	fmt.Printf("Imported %d setup keys\n", len(setupKeys))
// 	return nil
// }

// // generateSetupKeyResource generates a Terraform resource for a setup key
// func (s *SetupKeysGenerator) generateSetupKeyResource(setupKey SetupKey, groupIDToResourceName map[string]string) {
// 	resourceName := sanitizeResourceName(setupKey.Name)
// 	if resourceName == "" {
// 		resourceName = fmt.Sprintf("setup_key_%s", setupKey.ID)
// 	}

// 	// Convert auto_groups IDs to Terraform references
// 	autoGroupRefs := make([]string, 0)
// 	if len(setupKey.AutoGroups) > 0 {
// 		for _, groupID := range setupKey.AutoGroups {
// 			if groupResourceName, exists := groupIDToResourceName[groupID]; exists {
// 				terraformRef := fmt.Sprintf("netbird_group.%s.id", groupResourceName)
// 				autoGroupRefs = append(autoGroupRefs, terraformRef)
// 			} else {
// 				// Fallback to hardcoded ID if group not found in mapping
// 				autoGroupRefs = append(autoGroupRefs, groupID)
// 			}
// 		}
// 	}

// 	attributes := map[string]any{
// 		"id":          setupKey.ID,
// 		"name":        setupKey.Name,
// 		"type":        setupKey.Type,
// 		"expires_at":  setupKey.ExpiresAt,
// 		"revoked":     setupKey.Revoked,
// 		"usage_limit": setupKey.UsageLimit,
// 		"ephemeral":   setupKey.Ephemeral,
// 	}

// 	// Only add auto_groups if there are any
// 	if len(autoGroupRefs) > 0 {
// 		attributes["auto_groups"] = autoGroupRefs
// 	}

// 	if s.Generator != nil {
// 		s.Generator.AddResource("setup_key", resourceName, attributes)
// 	}
// }
