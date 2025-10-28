package main

import (
	"log"
	"os"
)

type Config struct {
	ServerURL  string
	APIToken   string
	Debug      bool
	AutoImport bool
}

func getConfig() *Config {
	serverURL := os.Getenv("NB_MANAGEMENT_URL")
	if serverURL == "" {
		serverURL = "https://netbird.api.com:33073"
	}

	if len(serverURL) > 0 && serverURL[len(serverURL)-1] == '/' {
		serverURL = serverURL[:len(serverURL)-1]
	}

	apiToken := os.Getenv("NB_PAT")
	if apiToken == "" {
		log.Fatal("NB_PAT environment variable is required (NetBird Personal Access Token)")
	}

	debug := os.Getenv("DEBUG") == "true"
	autoImport := os.Getenv("AUTO_IMPORT") != "false"

	return &Config{
		ServerURL:  serverURL,
		APIToken:   apiToken,
		Debug:      debug,
		AutoImport: autoImport,
	}
}
