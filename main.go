package main

import (
	"fmt"
	"log"
	"os"
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

	// Initialize service and terraform generator
	service := NewNetBirdService(config.ServerURL, config.APIToken, config.Debug)
	generator := NewTerraformGenerator(outputDir, config)

	// Step 1: Generate groups first to create the ID mapping
	groupsGen := &GroupsGenerator{Service: service, Generator: generator}
	err := groupsGen.InitResources()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// Get the group ID to resource name mapping
	groupIDToResourceName := groupsGen.GetGroupIDMapping()

	// Step 2: Generate other resources that may reference groups
	usersGen := &UsersGenerator{Service: service, Generator: generator}
	err = usersGen.InitResourcesWithGroupMapping(groupIDToResourceName)
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// setupKeysGen := &SetupKeysGenerator{Service: service, Generator: generator}
	// err = setupKeysGen.InitResourcesWithGroupMapping(groupIDToResourceName)
	// if err != nil {
	// 	fmt.Printf("Warning: %v\n", err)
	// }

	policiesGen := &PoliciesGenerator{Service: service, Generator: generator}
	err = policiesGen.InitResourcesWithGroupMapping(groupIDToResourceName)
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// Step 3: Generate other resources (routes already use proper references)
	remainingGenerators := []ResourceGenerator{
		&RoutesGenerator{Service: service, Generator: generator},
	}

	for _, gen := range remainingGenerators {
		err := gen.InitResources()
		if err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	// Generate Terraform files
	err = generator.GenerateFiles()
	if err != nil {
		log.Fatalf("Failed to generate Terraform files: %v", err)
	}

	// Generate group mapping file for reference
	err = generator.GenerateGroupMapping()
	if err != nil {
		log.Fatalf("Failed to generate group mapping: %v", err)
	}

	// Generate import script
	err = generator.GenerateImportScript()
	if err != nil {
		log.Fatalf("Failed to generate import script: %v", err)
	}

	// Run terraform imports if enabled
	if config.AutoImport {
		err = generator.RunTerraformImports()
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
	fmt.Printf("  - group_mappings.json (for ID reference)\n")
	fmt.Printf("  - import.sh (terraform import commands)\n")
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. cd %s\n", outputDir)
	if config.AutoImport {
		fmt.Printf("  2. terraform plan\n")
		fmt.Printf("  3. Review and modify the configuration as needed\n")
		fmt.Printf("\nNote: All resources have been automatically imported into Terraform state!\n")
	} else {
		fmt.Printf("  2. Run ./import.sh (or manually run terraform import commands)\n")
		fmt.Printf("  3. terraform plan\n")
		fmt.Printf("  4. Review and modify the configuration as needed\n")
	}
}

func showHelp() {
	fmt.Println("NetBird Standalone Terraform Importer")
	fmt.Println("=====================================")
	fmt.Println("")
	fmt.Println("Usage: ./netbird-importer [output-directory]")
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
	fmt.Println("  ./netbird-importer")
	fmt.Println("")
	fmt.Println("  # Import to custom directory with custom server")
	fmt.Println("  export NB_PAT=\"your-personal-access-token\"")
	fmt.Println("  export NB_MANAGEMENT_URL=\"https://netbird.monitorbit.xyz:33073\"")
	fmt.Println("  ./netbird-importer my-terraform-config")
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
	fmt.Println("  ./netbird-importer --debug-auth   # Test authentication")
}

func debugAuth() {
	fmt.Println("=== NetBird Authentication Debug ===")

	// Check environment variables
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

	// Check token format
	fmt.Printf("INFO: Token length: %d characters\n", len(pat))
	if len(pat) >= 10 {
		fmt.Printf("INFO: Token starts with: %s...\n", pat[:10])
	}

	// Test token format
	if len(pat) < 20 {
		fmt.Println("WARNING: Token seems unusually short")
	}

	// Create service and test
	fmt.Println("\n=== Testing API Connection ===")
	service := NewNetBirdService(managementURL, pat, true)

	var groups []Group
	err := service.Get("/api/groups", &groups)
	if err != nil {
		fmt.Printf("ERROR: API test failed: %v\n", err)
	} else {
		fmt.Printf("SUCCESS: API test successful! Found %d groups\n", len(groups))
	}
}
