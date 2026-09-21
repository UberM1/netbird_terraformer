package resources

import (
	"fmt"

	"netbird-terraformer/lib"
)

// MinVersion is the shape the API uses for per-OS version constraints
type MinVersion struct {
	MinVersion       string `json:"min_version"`
	MinKernelVersion string `json:"min_kernel_version"`
}

// OSVersionCheck represents per-OS minimum version requirements. The API nests
// these by OS; the provider flattens them into one attribute per OS.
type OSVersionCheck struct {
	Android *MinVersion `json:"android"`
	Darwin  *MinVersion `json:"darwin"`
	IOS     *MinVersion `json:"ios"`
	Linux   *MinVersion `json:"linux"`
	Windows *MinVersion `json:"windows"`
}

// GeoLocation represents a single location in a geo location check
type GeoLocation struct {
	CountryCode string `json:"country_code"`
	CityName    string `json:"city_name"`
}

// GeoLocationCheck represents a geographic restriction
type GeoLocationCheck struct {
	Action    string        `json:"action"`
	Locations []GeoLocation `json:"locations"`
}

// PeerNetworkRangeCheck represents a CIDR restriction
type PeerNetworkRangeCheck struct {
	Action string   `json:"action"`
	Ranges []string `json:"ranges"`
}

// Process represents a single process requirement
type Process struct {
	LinuxPath   string `json:"linux_path"`
	MacPath     string `json:"mac_path"`
	WindowsPath string `json:"windows_path"`
}

// ProcessCheck represents required running processes
type ProcessCheck struct {
	Processes []Process `json:"processes"`
}

// VersionCheck represents a minimum NetBird client version
type VersionCheck struct {
	MinVersion string `json:"min_version"`
}

// PostureChecks holds every check type a posture check can carry
type PostureChecks struct {
	NBVersionCheck        *VersionCheck          `json:"nb_version_check"`
	OSVersionCheck        *OSVersionCheck        `json:"os_version_check"`
	GeoLocationCheck      *GeoLocationCheck      `json:"geo_location_check"`
	PeerNetworkRangeCheck *PeerNetworkRangeCheck `json:"peer_network_range_check"`
	ProcessCheck          *ProcessCheck          `json:"process_check"`
}

// PostureCheck represents a NetBird posture check
type PostureCheck struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Checks      PostureChecks `json:"checks"`
}

// PostureChecksHandler implements ResourceHandler for posture checks
type PostureChecksHandler struct {
	service          lib.NetBirdAPI
	terraformWriter  lib.TerraformWriter
	idToResourceName map[string]string
}

// NewPostureChecksHandler creates a new posture checks handler
func NewPostureChecksHandler(service lib.NetBirdAPI, terraformWriter lib.TerraformWriter) *PostureChecksHandler {
	return &PostureChecksHandler{
		service:          service,
		terraformWriter:  terraformWriter,
		idToResourceName: make(map[string]string),
	}
}

// ImportAndGenerate imports posture checks from NetBird
func (h *PostureChecksHandler) ImportAndGenerate() error {
	fmt.Printf("Importing posture checks...\n")

	var checks []PostureCheck
	err := h.service.Get("/api/posture-checks", &checks)
	if err != nil {
		return fmt.Errorf("failed to fetch posture checks: %w", err)
	}

	for _, check := range checks {
		h.generatePostureCheckResource(check)
	}

	fmt.Printf("Imported %d posture checks\n", len(checks))
	return nil
}

// GetResourceMapping returns the posture check ID to resource name mapping,
// used by policies that reference a check through source_posture_checks
func (h *PostureChecksHandler) GetResourceMapping() map[string]string {
	return h.idToResourceName
}

// GetResourceType returns the resource type
func (h *PostureChecksHandler) GetResourceType() string {
	return "posture_check"
}

// generatePostureCheckResource generates a Terraform resource for a posture check
func (h *PostureChecksHandler) generatePostureCheckResource(check PostureCheck) {
	resourceName := lib.SanitizeResourceName(check.Name)
	if resourceName == "" {
		resourceName = fmt.Sprintf("posture_check_%s", check.ID)
	}

	attributes := map[string]any{
		"id":   check.ID,
		"name": check.Name,
		// The API always returns description, as "" when unset. Emitting it
		// unconditionally keeps the config aligned with what import writes to
		// state; dropping it makes every plan report "" -> null.
		"description": lib.ForceString(check.Description),
	}

	if check.Checks.NBVersionCheck != nil {
		// These are blocks in the provider schema, not attributes.
		attributes["netbird_version_check"] = map[string]any{
			"min_version": check.Checks.NBVersionCheck.MinVersion,
		}
	}

	if os := check.Checks.OSVersionCheck; os != nil {
		// The provider flattens the per-OS nesting the API returns.
		flattened := map[string]any{}
		if os.Android != nil {
			flattened["android_min_version"] = os.Android.MinVersion
		}
		if os.Darwin != nil {
			flattened["darwin_min_version"] = os.Darwin.MinVersion
		}
		if os.IOS != nil {
			flattened["ios_min_version"] = os.IOS.MinVersion
		}
		if os.Linux != nil {
			flattened["linux_min_kernel_version"] = os.Linux.MinKernelVersion
		}
		if os.Windows != nil {
			flattened["windows_min_kernel_version"] = os.Windows.MinKernelVersion
		}
		if len(flattened) > 0 {
			attributes["os_version_check"] = flattened
		}
	}

	if geo := check.Checks.GeoLocationCheck; geo != nil {
		locations := make(lib.ObjectList, 0, len(geo.Locations))
		for _, location := range geo.Locations {
			entry := map[string]any{"country_code": location.CountryCode}
			if location.CityName != "" {
				entry["city_name"] = location.CityName
			}
			locations = append(locations, entry)
		}
		attributes["geo_location_check"] = map[string]any{
			"action":    geo.Action,
			"locations": locations,
		}
	}

	if ranges := check.Checks.PeerNetworkRangeCheck; ranges != nil {
		attributes["peer_network_range_check"] = map[string]any{
			"action": ranges.Action,
			"ranges": ranges.Ranges,
		}
	}

	if process := check.Checks.ProcessCheck; process != nil && len(process.Processes) > 0 {
		processes := make([]any, 0, len(process.Processes))
		for _, entry := range process.Processes {
			mapped := map[string]any{}
			if entry.LinuxPath != "" {
				mapped["linux_path"] = entry.LinuxPath
			}
			if entry.MacPath != "" {
				mapped["mac_path"] = entry.MacPath
			}
			if entry.WindowsPath != "" {
				mapped["windows_path"] = entry.WindowsPath
			}
			processes = append(processes, mapped)
		}
		attributes["process_check"] = processes
	}

	h.idToResourceName[check.ID] = resourceName
	h.terraformWriter.AddResource("posture_check", resourceName, attributes)
}
