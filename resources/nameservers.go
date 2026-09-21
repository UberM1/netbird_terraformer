package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// Nameserver represents a single resolver inside a nameserver group
type Nameserver struct {
	IP     string `json:"ip"`
	NSType string `json:"ns_type"`
	Port   int    `json:"port"`
}

// NameserverGroup represents a NetBird nameserver group
type NameserverGroup struct {
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	Description          string       `json:"description"`
	Nameservers          []Nameserver `json:"nameservers"`
	Enabled              bool         `json:"enabled"`
	Groups               []string     `json:"groups"`
	Primary              bool         `json:"primary"`
	Domains              []string     `json:"domains"`
	SearchDomainsEnabled bool         `json:"search_domains_enabled"`
}

// DNSSettings represents the account-wide DNS settings singleton
type DNSSettings struct {
	DisabledManagementGroups []string `json:"disabled_management_groups"`
}

// DNSRecord represents a record inside a DNS zone
type DNSRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

// DNSZone represents a NetBird internal DNS zone
type DNSZone struct {
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
	Domain             string      `json:"domain"`
	Enabled            bool        `json:"enabled"`
	EnableSearchDomain bool        `json:"enable_search_domain"`
	DistributionGroups []string    `json:"distribution_groups"`
	Records            []DNSRecord `json:"records"`
}

// NameserversHandler implements ResourceHandler for DNS nameserver groups and settings
type NameserversHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	groupMapping    map[string]string
}

// NewNameserversHandler creates a new nameservers handler
func NewNameserversHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *NameserversHandler {
	return &NameserversHandler{
		service:         service,
		terraformWriter: terraformWriter,
		groupMapping:    make(map[string]string),
	}
}

// SetGroupMapping sets the group ID to resource name mapping
func (h *NameserversHandler) SetGroupMapping(groupMapping map[string]string) {
	h.groupMapping = groupMapping
}

// ImportAndGenerate imports nameserver groups and DNS settings from NetBird
func (h *NameserversHandler) ImportAndGenerate() error {
	fmt.Printf("Importing nameserver groups...\n")

	var groups []NameserverGroup
	err := h.service.Get("/api/dns/nameservers", &groups)
	if err != nil {
		return fmt.Errorf("failed to fetch nameserver groups: %w", err)
	}

	for _, group := range groups {
		h.generateNameserverGroupResource(group)
	}

	fmt.Printf("Imported %d nameserver groups\n", len(groups))

	var settings DNSSettings
	err = h.service.Get("/api/dns/settings", &settings)
	if err != nil {
		return fmt.Errorf("failed to fetch dns settings: %w", err)
	}

	h.generateDNSSettingsResource(settings)

	return h.importZones()
}

// importZones imports internal DNS zones and the records inside them
func (h *NameserversHandler) importZones() error {
	fmt.Printf("Importing DNS zones...\n")

	var zones []DNSZone
	err := h.service.Get("/api/dns/zones", &zones)
	if err != nil {
		return fmt.Errorf("failed to fetch dns zones: %w", err)
	}

	records := 0
	for _, zone := range zones {
		zoneResourceName := h.generateDNSZoneResource(zone)
		for _, record := range zone.Records {
			h.generateDNSRecordResource(zone, zoneResourceName, record)
		}
		records += len(zone.Records)
	}

	fmt.Printf("Imported %d DNS zones, %d records\n", len(zones), records)
	return nil
}

// generateDNSZoneResource generates a Terraform resource for a DNS zone
func (h *NameserversHandler) generateDNSZoneResource(zone DNSZone) string {
	resourceName := lib.SanitizeResourceName(zone.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("dns_zone_%s", zone.ID)
	}

	attributes := map[string]any{
		"id":                   zone.ID,
		"name":                 zone.Name,
		"domain":               zone.Domain,
		"enabled":              zone.Enabled,
		"enable_search_domain": zone.EnableSearchDomain,
		"distribution_groups":  h.groupRefs(zone.DistributionGroups),
	}

	h.terraformWriter.AddResource("dns_zone", resourceName, attributes)
	return resourceName
}

// generateDNSRecordResource generates a Terraform resource for a DNS record
func (h *NameserversHandler) generateDNSRecordResource(zone DNSZone, zoneResourceName string, record DNSRecord) {
	resourceName := fmt.Sprintf("%s_%s", zoneResourceName, lib.SanitizeResourceName(record.Name))

	attributes := map[string]any{
		// This resource imports as zone_id:record_id.
		"id":      fmt.Sprintf("%s:%s", zone.ID, record.ID),
		"name":    record.Name,
		"type":    record.Type,
		"content": record.Content,
		"ttl":     record.TTL,
		"zone_id": lib.RawValue(fmt.Sprintf("netbird_dns_zone.%s.id", zoneResourceName)),
	}

	h.terraformWriter.AddResource("dns_record", resourceName, attributes)
}

// GetResourceMapping returns an empty mapping since nameserver groups are not referenced
func (h *NameserversHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *NameserversHandler) GetResourceType() string {
	return "nameserver_group"
}

// groupRefs converts group IDs into Terraform references
func (h *NameserversHandler) groupRefs(ids []string) []string {
	refs := make([]string, 0, len(ids))
	for _, id := range ids {
		if resourceName, exists := h.groupMapping[id]; exists {
			refs = append(refs, lib.CreateTerraformReference("group", resourceName))
		}
	}
	return refs
}

// generateNameserverGroupResource generates a Terraform resource for a nameserver group
func (h *NameserversHandler) generateNameserverGroupResource(group NameserverGroup) {
	resourceName := lib.SanitizeResourceName(group.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("nameserver_group_%s", group.ID)
	}

	// "nameservers" is an attributes list in the provider schema, not a block list.
	nameservers := make(lib.ObjectList, 0, len(group.Nameservers))
	for _, ns := range group.Nameservers {
		nameservers = append(nameservers, map[string]any{
			"ip":      ns.IP,
			"ns_type": ns.NSType,
			"port":    ns.Port,
		})
	}

	attributes := map[string]any{
		"id":                     group.ID,
		"name":                   group.Name,
		"description":            group.Description,
		"enabled":                group.Enabled,
		"primary":                group.Primary,
		"search_domains_enabled": group.SearchDomainsEnabled,
		"nameservers":            nameservers,
		"groups":                 h.groupRefs(group.Groups),
	}

	// Only valid when primary is false.
	if len(group.Domains) > 0 {
		attributes["domains"] = group.Domains
	}

	h.terraformWriter.AddResource("nameserver_group", resourceName, attributes)
}

// generateDNSSettingsResource generates the account-wide DNS settings resource
func (h *NameserversHandler) generateDNSSettingsResource(settings DNSSettings) {
	refs := h.groupRefs(settings.DisabledManagementGroups)

	// The field is required, so an empty list still has to be written out.
	var value any = refs
	if len(refs) == 0 {
		value = lib.RawValue("[]")
	}

	attributes := map[string]any{
		"disabled_management_groups": value,
	}

	// The provider documents no import syntax for this singleton.
	h.terraformWriter.AddResourceNoImport("dns_settings", "this", attributes)
}
