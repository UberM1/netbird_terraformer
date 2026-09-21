package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// ReverseProxyDomain represents a domain served by a reverse proxy cluster
type ReverseProxyDomain struct {
	ID            string `json:"id"`
	Domain        string `json:"domain"`
	Type          string `json:"type"`
	TargetCluster string `json:"target_cluster"`
	Validated     bool   `json:"validated"`
}

// ReverseProxyTargetOptions represents per-target tuning.
//
// custom_headers is deliberately not mapped: the API marks it sensitive and it
// must never be written to a .tf file or into Terraform state.
type ReverseProxyTargetOptions struct {
	RequestTimeout     string `json:"request_timeout"`
	PathRewrite        string `json:"path_rewrite"`
	SkipTLSVerify      bool   `json:"skip_tls_verify"`
	ProxyProtocol      bool   `json:"proxy_protocol"`
	SessionIdleTimeout string `json:"session_idle_timeout"`
}

// ReverseProxyTarget represents a backend behind a reverse proxy service
type ReverseProxyTarget struct {
	TargetID   string                     `json:"target_id"`
	TargetType string                     `json:"target_type"`
	Port       int                        `json:"port"`
	Protocol   string                     `json:"protocol"`
	Host       string                     `json:"host"`
	Path       string                     `json:"path"`
	Enabled    bool                       `json:"enabled"`
	Options    *ReverseProxyTargetOptions `json:"options"`
}

// AuthToggle is an auth method that only carries an enabled flag.
// Secret fields (password, pin) are intentionally not mapped.
type AuthToggle struct {
	Enabled bool `json:"enabled"`
}

// BearerAuth represents bearer token authentication
type BearerAuth struct {
	Enabled            bool     `json:"enabled"`
	DistributionGroups []string `json:"distribution_groups"`
}

// HeaderAuth represents header based authentication. The header value is
// sensitive and is intentionally not mapped.
type HeaderAuth struct {
	Enabled bool   `json:"enabled"`
	Header  string `json:"header"`
}

// ReverseProxyAuth represents the authentication configuration of a service
type ReverseProxyAuth struct {
	PasswordAuth *AuthToggle  `json:"password_auth"`
	PinAuth      *AuthToggle  `json:"pin_auth"`
	LinkAuth     *AuthToggle  `json:"link_auth"`
	BearerAuth   *BearerAuth  `json:"bearer_auth"`
	HeaderAuths  []HeaderAuth `json:"header_auths"`
}

// ReverseProxyAccessRestrictions represents geo and CIDR filtering
type ReverseProxyAccessRestrictions struct {
	AllowedCountries []string `json:"allowed_countries"`
	BlockedCountries []string `json:"blocked_countries"`
	AllowedCIDRs     []string `json:"allowed_cidrs"`
	BlockedCIDRs     []string `json:"blocked_cidrs"`
}

// ReverseProxyService represents a NetBird reverse proxy service
type ReverseProxyService struct {
	ID                 string                          `json:"id"`
	Name               string                          `json:"name"`
	Domain             string                          `json:"domain"`
	Mode               string                          `json:"mode"`
	Enabled            bool                            `json:"enabled"`
	ListenPort         int                             `json:"listen_port"`
	PassHostHeader     bool                            `json:"pass_host_header"`
	RewriteRedirects   bool                            `json:"rewrite_redirects"`
	Targets            []ReverseProxyTarget            `json:"targets"`
	Auth               *ReverseProxyAuth               `json:"auth"`
	AccessRestrictions *ReverseProxyAccessRestrictions `json:"access_restrictions"`
}

// ReverseProxiesHandler implements ResourceHandler for reverse proxy resources
type ReverseProxiesHandler struct {
	service         lib.NetBirdAPI
	terraformWriter lib.TerraformWriter
	groupMapping    map[string]string
	references      *ReferenceResolver
}

// SetReferenceResolver sets the resolver used for target IDs
func (h *ReverseProxiesHandler) SetReferenceResolver(resolver *ReferenceResolver) {
	h.references = resolver
}

// NewReverseProxiesHandler creates a new reverse proxies handler
func NewReverseProxiesHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *ReverseProxiesHandler {
	return &ReverseProxiesHandler{
		service:         service,
		terraformWriter: terraformWriter,
		groupMapping:    make(map[string]string),
	}
}

// SetGroupMapping sets the group ID to resource name mapping
func (h *ReverseProxiesHandler) SetGroupMapping(groupMapping map[string]string) {
	h.groupMapping = groupMapping
}

// ImportAndGenerate imports reverse proxy domains and services
func (h *ReverseProxiesHandler) ImportAndGenerate() error {
	fmt.Printf("Importing reverse proxy domains...\n")

	var domains []ReverseProxyDomain
	err := h.service.Get("/api/reverse-proxies/domains", &domains)
	if err != nil {
		return fmt.Errorf("failed to fetch reverse proxy domains: %w", err)
	}

	managed := 0
	for _, domain := range domains {
		// Free cluster domains are built in: they carry no ID and cannot be managed.
		if domain.ID == "" || domain.Type != "custom" {
			continue
		}
		h.generateDomainResource(domain)
		managed++
	}
	fmt.Printf("Imported %d custom reverse proxy domains (%d built-in skipped)\n", managed, len(domains)-managed)

	fmt.Printf("Importing reverse proxy services...\n")

	var services []ReverseProxyService
	err = h.service.Get("/api/reverse-proxies/services", &services)
	if err != nil {
		return fmt.Errorf("failed to fetch reverse proxy services: %w", err)
	}

	for _, svc := range services {
		h.generateServiceResource(svc)
	}
	fmt.Printf("Imported %d reverse proxy services\n", len(services))

	return nil
}

// GetResourceMapping returns an empty mapping since these are not referenced
func (h *ReverseProxiesHandler) GetResourceMapping() map[string]string {
	return make(map[string]string)
}

// GetResourceType returns the resource type
func (h *ReverseProxiesHandler) GetResourceType() string {
	return "reverse_proxy_service"
}

// generateDomainResource generates a Terraform resource for a custom domain
func (h *ReverseProxiesHandler) generateDomainResource(domain ReverseProxyDomain) {
	resourceName := lib.SanitizeResourceName(domain.Domain)
	if resourceName == "" {
		resourceName = fmt.Sprintf("domain_%s", domain.ID)
	}

	// type and validated are read-only.
	attributes := map[string]any{
		"id":     domain.ID,
		"domain": domain.Domain,
	}

	// Reference the cluster through the lookup map in clusters.tf rather than
	// inlining its address. The map is keyed by address, so a cluster that is
	// renamed or decommissioned fails the plan instead of silently retargeting
	// the domain. Fall back to the literal if the address is unknown, since a
	// reference to a missing key would not even parse.
	if domain.TargetCluster != "" {
		attributes["target_cluster"] = lib.RawValue(
			fmt.Sprintf("local.proxy_clusters[%q].address", domain.TargetCluster),
		)
	} else {
		attributes["target_cluster"] = domain.TargetCluster
	}

	h.terraformWriter.AddResource("reverse_proxy_domain", resourceName, attributes)
}

// targetRef resolves a target ID into a reference when possible
func (h *ReverseProxiesHandler) targetRef(target ReverseProxyTarget) any {
	if h.references == nil {
		return target.TargetID
	}
	return h.references.Reference(target.TargetID, target.TargetType)
}

// generateServiceResource generates a Terraform resource for a proxy service
func (h *ReverseProxiesHandler) generateServiceResource(svc ReverseProxyService) {
	resourceName := lib.SanitizeResourceName(svc.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("service_%s", svc.ID)
	}

	// The provider models these as attributes, not blocks.
	targets := make(lib.ObjectList, 0, len(svc.Targets))
	for _, target := range svc.Targets {
		targetMap := map[string]any{
			"target_id":   h.targetRef(target),
			"target_type": target.TargetType,
			"port":        target.Port,
			"protocol":    target.Protocol,
			"enabled":     target.Enabled,
		}
		if target.Host != "" {
			targetMap["host"] = target.Host
		}
		if target.Path != "" {
			targetMap["path"] = target.Path
		}
		if target.Options != nil {
			// Only emit fields that are actually set upstream. The API returns
			// "options": {} for a target with no tuning, which unmarshals to a
			// non-nil pointer to a zero struct -- writing the zero values out
			// explicitly makes Terraform compare {} against
			// {proxy_protocol = false, skip_tls_verify = false} and report a
			// permanent no-op diff on every plan.
			options := lib.ObjectValue{}
			if target.Options.SkipTLSVerify {
				options["skip_tls_verify"] = true
			}
			if target.Options.ProxyProtocol {
				options["proxy_protocol"] = true
			}
			if target.Options.RequestTimeout != "" {
				options["request_timeout"] = target.Options.RequestTimeout
			}
			if target.Options.PathRewrite != "" {
				options["path_rewrite"] = target.Options.PathRewrite
			}
			if target.Options.SessionIdleTimeout != "" {
				options["session_idle_timeout"] = target.Options.SessionIdleTimeout
			}
			if len(options) > 0 {
				targetMap["options"] = options
			}
		}
		targets = append(targets, targetMap)
	}

	// auth is required, so an empty configuration still has to be emitted.
	authValue := h.authAttributes(svc)
	var auth any = authValue
	if len(authValue) == 0 {
		auth = lib.RawValue("{}")
	}

	// port_auto_assigned, proxy_cluster and private are read-only.
	attributes := map[string]any{
		"id":                svc.ID,
		"name":              svc.Name,
		"domain":            svc.Domain,
		"mode":              svc.Mode,
		"enabled":           svc.Enabled,
		"listen_port":       svc.ListenPort,
		"pass_host_header":  svc.PassHostHeader,
		"rewrite_redirects": svc.RewriteRedirects,
		"targets":           targets,
		"auth":              auth,
	}

	if svc.AccessRestrictions != nil {
		restrictions := lib.ObjectValue{}
		if len(svc.AccessRestrictions.AllowedCountries) > 0 {
			restrictions["allowed_countries"] = svc.AccessRestrictions.AllowedCountries
		}
		if len(svc.AccessRestrictions.BlockedCountries) > 0 {
			restrictions["blocked_countries"] = svc.AccessRestrictions.BlockedCountries
		}
		if len(svc.AccessRestrictions.AllowedCIDRs) > 0 {
			restrictions["allowed_cidrs"] = svc.AccessRestrictions.AllowedCIDRs
		}
		if len(svc.AccessRestrictions.BlockedCIDRs) > 0 {
			restrictions["blocked_cidrs"] = svc.AccessRestrictions.BlockedCIDRs
		}
		if len(restrictions) > 0 {
			attributes["access_restrictions"] = restrictions
		}
	}

	h.terraformWriter.AddResource("reverse_proxy_service", resourceName, attributes)
}

// authAttributes builds the auth block. Secrets are never exported, so any
// enabled secret-bearing method is reported for manual wiring.
func (h *ReverseProxiesHandler) authAttributes(svc ReverseProxyService) lib.ObjectValue {
	auth := lib.ObjectValue{}
	if svc.Auth == nil {
		return auth
	}

	if svc.Auth.PasswordAuth != nil {
		passwordAuth := lib.ObjectValue{"enabled": svc.Auth.PasswordAuth.Enabled}
		if svc.Auth.PasswordAuth.Enabled {
			// A real password is set upstream but never exported. Leaving the
			// field out keeps Terraform from overwriting it with an empty one.
			fmt.Printf("  Warning: service %q uses password auth; the password is not exported, wire it through a variable\n", svc.Name)
		} else {
			// Disabled auth carries "" upstream. Emit it so the config matches
			// state instead of planning a permanent "" -> null update.
			passwordAuth["password"] = lib.ForceString("")
		}
		auth["password_auth"] = passwordAuth
	}
	if svc.Auth.PinAuth != nil {
		pinAuth := lib.ObjectValue{"enabled": svc.Auth.PinAuth.Enabled}
		if svc.Auth.PinAuth.Enabled {
			fmt.Printf("  Warning: service %q uses PIN auth; the PIN is not exported, wire it through a variable\n", svc.Name)
		} else {
			pinAuth["pin"] = lib.ForceString("")
		}
		auth["pin_auth"] = pinAuth
	}
	if svc.Auth.LinkAuth != nil {
		auth["link_auth"] = lib.ObjectValue{"enabled": svc.Auth.LinkAuth.Enabled}
	}
	if svc.Auth.BearerAuth != nil {
		bearer := lib.ObjectValue{"enabled": svc.Auth.BearerAuth.Enabled}
		refs := make([]string, 0, len(svc.Auth.BearerAuth.DistributionGroups))
		for _, id := range svc.Auth.BearerAuth.DistributionGroups {
			if resourceName, exists := h.groupMapping[id]; exists {
				refs = append(refs, lib.CreateTerraformReference("group", resourceName))
			}
		}
		if len(refs) > 0 {
			bearer["distribution_groups"] = refs
		}
		auth["bearer_auth"] = bearer
	}
	if len(svc.Auth.HeaderAuths) > 0 {
		headers := make(lib.ObjectList, 0, len(svc.Auth.HeaderAuths))
		for _, header := range svc.Auth.HeaderAuths {
			headers = append(headers, map[string]any{
				"enabled": header.Enabled,
				"header":  header.Header,
			})
			if header.Enabled {
				fmt.Printf("  Warning: service %q uses header auth %q; the value is not exported, wire it through a variable\n", svc.Name, header.Header)
			}
		}
		auth["header_auths"] = headers
	}

	return auth
}
