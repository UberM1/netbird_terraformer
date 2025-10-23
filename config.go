package main

import (
	"log"
	"os"
)

// Config holds the configuration for the NetBird importer
type Config struct {
	ServerURL string
	APIToken  string
	Debug     bool
}

// getConfig reads configuration from environment variables
func getConfig() *Config {
	// Use the same environment variables as the official NetBird Terraform provider
	serverURL := os.Getenv("NB_MANAGEMENT_URL")
	if serverURL == "" {
		serverURL = "https://netbird.monitorbit.xyz:33073"
	}

	// Remove trailing slash if present
	if len(serverURL) > 0 && serverURL[len(serverURL)-1] == '/' {
		serverURL = serverURL[:len(serverURL)-1]
	}

	apiToken := os.Getenv("NB_PAT")
	if apiToken == "" {
		log.Fatal("NB_PAT environment variable is required (NetBird Personal Access Token)")
	}

	debug := os.Getenv("DEBUG") == "true"

	return &Config{
		ServerURL: serverURL,
		APIToken:  apiToken,
		Debug:     debug,
	}
}
