package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Policy represents a NetBird policy
type Policy struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Enabled             bool         `json:"enabled"`
	Rules               []PolicyRule `json:"rules"`
	SourcePostureChecks []string     `json:"source_posture_checks"`
}

// PolicyRule represents a rule within a policy
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

// Resource represents a NetBird resource
type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Handler implements ResourceHandler for policies
type PoliciesHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	groupMapping    map[string]string
}

// NewHandler creates a new policies handler
func NewPoliciesHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *PoliciesHandler {
	return &PoliciesHandler{
		service:         service,
		terraformWriter: terraformWriter,
		groupMapping:    make(map[string]string),
	}
}

// SetGroupMapping sets the group ID to resource name mapping
func (h *PoliciesHandler) SetGroupMapping(groupMapping map[string]string) {
	h.groupMapping = groupMapping
}

// ImportAndGenerate imports policies from NetBird and generates Terraform resources
func (h *PoliciesHandler) ImportAndGenerate() error {
	fmt.Printf("Importing policies...\n")

	var policies []Policy
	err := h.service.Get("/api/policies", &policies)
	if err != nil {
		return fmt.Errorf("failed to fetch policies: %w", err)
	}

	for _, policy := range policies {
		h.generatePolicyResource(policy)
	}

	fmt.Printf("Imported %d policies\n", len(policies))
	return nil
}

// GetResourceMapping returns an empty mapping since policies don't need to be referenced
func (h *PoliciesHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *PoliciesHandler) GetResourceType() string {
	return "policy"
}

// generatePolicyResource generates a Terraform resource for a policy
func (h *PoliciesHandler) generatePolicyResource(policy Policy) {
	resourceName := lib.SanitizeResourceName(policy.Name)
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
		rules := make([]any, 0)
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
					if groupResourceName, exists := h.groupMapping[source.ID]; exists {
						terraformRef := lib.CreateTerraformReference("group", groupResourceName)
						sources = append(sources, terraformRef)
					} else {
						groupResourceName := lib.SanitizeResourceName(source.Name)
						terraformRef := lib.CreateTerraformReference("group", groupResourceName)
						sources = append(sources, terraformRef)
					}
				}
				ruleMap["sources"] = sources
			}

			// Handle destinations - use proper group mapping
			if len(rule.Destinations) > 0 {
				destinations := make([]string, 0)
				for _, dest := range rule.Destinations {
					if groupResourceName, exists := h.groupMapping[dest.ID]; exists {
						terraformRef := lib.CreateTerraformReference("group", groupResourceName)
						destinations = append(destinations, terraformRef)
					} else {
						groupResourceName := lib.SanitizeResourceName(dest.Name)
						terraformRef := lib.CreateTerraformReference("group", groupResourceName)
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

	h.terraformWriter.AddResource("policy", resourceName, attributes)
}
