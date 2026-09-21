package main

import (
	"fmt"
	"log"
	"os"

	"netbird-terraformer/lib"
	"netbird-terraformer/resources"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		showHelp()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--debug-auth" {
		debugAuth()
		return
	}

	// Get configuration
	config := getConfig()

	// Create output directory
	outputDir := "generated"
	if len(os.Args) > 1 {
		outputDir = os.Args[1]
	}

	fmt.Printf("NetBird Terraform Importer\n")
	fmt.Printf("Server URL: %s\n", config.ServerURL)
	fmt.Printf("Output Directory: %s\n", outputDir)
	fmt.Printf("Starting import...\n\n")

	// Create service and terraform generator
	service := NewNetBirdService(config.ServerURL, config.APIToken, config.Debug)
	terraformGen := lib.NewTerraformGenerator(outputDir, &lib.Config{
		ServerURL:  config.ServerURL,
		APIToken:   config.APIToken,
		Debug:      config.Debug,
		AutoImport: config.AutoImport,
	})

	// Initialize resource handlers
	groupsHandler := resources.NewGroupsHandler(service, terraformGen)
	usersHandler := resources.NewUsersHandler(service, terraformGen)
	policiesHandler := resources.NewPoliciesHandler(service, terraformGen)
	routesHandler := resources.NewRoutesHandler(service, terraformGen)
	nameserversHandler := resources.NewNameserversHandler(service, terraformGen)
	networksHandler := resources.NewNetworksHandler(service, terraformGen)
	reverseProxiesHandler := resources.NewReverseProxiesHandler(service, terraformGen)
	postureChecksHandler := resources.NewPostureChecksHandler(service, terraformGen)
	peerResolver := resources.NewPeerResolver(service, terraformGen)

	// Peers are referenced by routes and network routers, so resolve them first.
	if err := peerResolver.Load(); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// Import groups first to establish group mappings
	err := groupsHandler.ImportAndGenerate()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// Get group mappings for other resources
	groupMapping := groupsHandler.GetResourceMapping()

	// Set group mapping for resources that need it
	usersHandler.SetGroupMapping(groupMapping)
	policiesHandler.SetGroupMapping(groupMapping)
	nameserversHandler.SetGroupMapping(groupMapping)
	networksHandler.SetGroupMapping(groupMapping)
	reverseProxiesHandler.SetGroupMapping(groupMapping)
	routesHandler.SetPeerResolver(peerResolver)
	networksHandler.SetPeerResolver(peerResolver)

	referenceResolver := resources.NewReferenceResolver(peerResolver, networksHandler)
	policiesHandler.SetReferenceResolver(referenceResolver)
	policiesHandler.SetPostureCheckMapping(postureChecksHandler.GetResourceMapping())
	reverseProxiesHandler.SetReferenceResolver(referenceResolver)

	// Import other resources
	// Networks run before policies and reverse proxies: both reference network
	// resources, and the mapping only exists once those are generated.
	resourceHandlers := []lib.ResourceHandler{
		usersHandler,
		networksHandler,
		postureChecksHandler,
		policiesHandler,
		routesHandler,
		nameserversHandler,
		reverseProxiesHandler,
	}

	for _, handler := range resourceHandlers {
		err := handler.ImportAndGenerate()
		if err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	// Generate files and scripts
	err = generateTerraformFiles(terraformGen, outputDir)
	if err != nil {
		log.Fatalf("Failed to generate Terraform files: %v", err)
	}

	// Must be written whenever a reverse proxy domain is generated: the domain's
	// target_cluster references the lookup map this file defines.
	err = terraformGen.GenerateClustersFile()
	if err != nil {
		log.Fatalf("Failed to generate clusters file: %v", err)
	}

	err = terraformGen.GenerateGroupMapping()
	if err != nil {
		log.Fatalf("Failed to generate group mapping: %v", err)
	}

	err = terraformGen.GenerateImportScript()
	if err != nil {
		log.Fatalf("Failed to generate import script: %v", err)
	}

	err = terraformGen.GenerateImportBlocks()
	if err != nil {
		log.Fatalf("Failed to generate import blocks: %v", err)
	}

	// Handle imports
	if config.AutoImport {
		err = runTerraformImports(terraformGen, outputDir)
		if err != nil {
			log.Fatalf("Failed to run terraform imports: %v", err)
		}
	} else {
		fmt.Printf("\nAuto-import disabled. You can manually run terraform imports later.\n")
	}

	fmt.Printf("\nImport completed successfully!\n")
	fmt.Printf("Generated files in: %s\n", outputDir)
	fmt.Printf("\nFiles generated:\n")
	fmt.Printf("  - Terraform configuration files (*.tf)\n")
	fmt.Printf("  - imports.tf (terraform import blocks)\n")
	fmt.Printf("  - clusters.tf (reverse proxy cluster lookup)\n")
	fmt.Printf("  - group_mappings.json (for ID reference)\n")
	fmt.Printf("  - import.sh (legacy, superseded by imports.tf)\n")
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. cd %s\n", outputDir)
	if config.AutoImport {
		fmt.Printf("  2. terraform plan\n")
		fmt.Printf("  3. Review and modify the configuration as needed\n")
		fmt.Printf("\nNote: All resources have been automatically imported into Terraform state!\n")
	} else {
		fmt.Printf("  2. Prune imports.tf against the state: terraform state list > state.txt\n")
		fmt.Printf("     then: bash prune_imports.sh imports.tf state.txt\n")
		fmt.Printf("  3. terraform plan   (review what would be imported)\n")
		fmt.Printf("  4. terraform apply  (binds the resources, then delete imports.tf)\n")
	}
}

// generateTerraformFiles groups resources by type and generates .tf files
func generateTerraformFiles(terraformGen *lib.TerraformGenerator, outputDir string) error {
	fmt.Printf("\nGenerating Terraform files...\n")

	// First generate provider.tf
	err := terraformGen.GenerateProviderFile()
	if err != nil {
		return fmt.Errorf("failed to generate provider file: %w", err)
	}

	// provider.tf references these, so the output is not valid without them
	err = terraformGen.GenerateVariablesFile()
	if err != nil {
		return fmt.Errorf("failed to generate variables file: %w", err)
	}

	// Group resources by type and generate files
	resources := terraformGen.GetResources()
	resourcesByType := make(map[string][]lib.TerraformResource)
	for _, resource := range resources {
		resourcesByType[resource.Type] = append(resourcesByType[resource.Type], resource)
	}

	// Generate a file for each resource type
	for resourceType, resources := range resourcesByType {
		fmt.Printf("Generating %s.tf with %d resources...\n", resourceType, len(resources))
		err := terraformGen.WriteResourceFile(resourceType, resources)
		if err != nil {
			return fmt.Errorf("failed to generate %s resources: %w", resourceType, err)
		}
	}

	fmt.Printf("Terraform files generated successfully\n")
	return nil
}

// runTerraformImports executes terraform init and import commands
func runTerraformImports(terraformGen *lib.TerraformGenerator, outputDir string) error {
	importCommands := terraformGen.GetImportCommands()
	if len(importCommands) == 0 {
		fmt.Printf("No terraform imports to run\n")
		return nil
	}

	fmt.Printf("\nRunning terraform imports...\n")

	fmt.Printf("Running terraform init...\n")
	err := lib.TerraformInit(outputDir)
	if err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	successCount := 0
	for _, cmd := range importCommands {
		fmt.Printf("Importing %s...\n", cmd.ResourceAddress)
		err := lib.TerraformImport(outputDir, cmd.ResourceAddress, cmd.ResourceID)
		if err != nil {
			fmt.Printf("  Warning: terraform import failed for %s: %v\n", cmd.ResourceAddress, err)
		} else {
			fmt.Printf("  Successfully imported %s\n", cmd.ResourceAddress)
			successCount++
		}
	}

	fmt.Printf("\nTerraform import completed: %d/%d successful\n", successCount, len(importCommands))
	return nil
}

func showHelp() {
	fmt.Println("NetBird terraformer Terraform Importer")
	fmt.Println("=====================================")
	fmt.Println("")
	fmt.Println("Usage: ./netbird-terraformer [output-directory]")
	fmt.Println("")
	fmt.Println("Environment variables:")
	fmt.Println("  NB_PAT                - Your NetBird Personal Access Token (required)")
	fmt.Println("  NB_MANAGEMENT_URL     - NetBird Management API URL (optional)")
	fmt.Println("                          Defaults to https://api.netbird.io")
	fmt.Println("  DEBUG                 - Enable debug output (optional, set to 'true')")
	fmt.Println("  AUTO_IMPORT           - Auto-run terraform import (optional, set to 'false' to disable)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Import to default 'generated' directory")
	fmt.Println("  export NB_PAT=\"your-personal-access-token\"")
	fmt.Println("  ./netbird-terraformer")
	fmt.Println("")
	fmt.Println("  # Import to custom directory with custom server")
	fmt.Println("  export NB_PAT=\"your-personal-access-token\"")
	fmt.Println("  export NB_MANAGEMENT_URL=\"https://netbird.example.com:33073\"")
	fmt.Println("  ./netbird-terraformer my-terraform-config")
	fmt.Println("")
	fmt.Println("Resource types imported:")
	fmt.Println("  - Groups")
	fmt.Println("  - Users")
	fmt.Println("  - Policies")
	fmt.Println("  - Routes")
	fmt.Println("  - Setup Keys")
	fmt.Println("")
	fmt.Println("Note: Peers are managed by the NetBird client and available as data sources only.")
	fmt.Println("")
	fmt.Println("Debug commands:")
	fmt.Println("  ./netbird-terraformer --debug-auth   # Test authentication")
}

func debugAuth() {
	fmt.Println("=== NetBird Authentication Debug ===")

	pat := os.Getenv("NB_PAT")
	managementURL := os.Getenv("NB_MANAGEMENT_URL")

	if pat == "" {
		fmt.Println("ERROR: NB_PAT is not set")
		return
	}

	if managementURL == "" {
		managementURL = "https://api.netbird.io"
		fmt.Printf("INFO: Using default management URL: %s\n", managementURL)
	} else {
		fmt.Printf("INFO: Using custom management URL: %s\n", managementURL)
	}

	fmt.Printf("INFO: Token length: %d characters\n", len(pat))
	if len(pat) >= 10 {
		fmt.Printf("INFO: Token starts with: %s...\n", pat[:10])
	}

	if len(pat) < 20 {
		fmt.Println("WARNING: Token seems unusually short")
	}

	fmt.Println("\n=== Testing API Connection ===")
	service := NewNetBirdService(managementURL, pat, true)

	var groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	err := service.Get("/api/groups", &groups)
	if err != nil {
		fmt.Printf("ERROR: API test failed: %v\n", err)
	} else {
		fmt.Printf("SUCCESS: API test successful! Found %d groups\n", len(groups))
	}
}
