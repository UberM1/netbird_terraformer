package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type NetBirdService struct {
	apiEndpoint string
	apiToken    string
	client      *http.Client
	debug       bool
}

func NewNetBirdService(apiEndpoint, apiToken string, debug bool) *NetBirdService {
	return &NetBirdService{
		apiEndpoint: apiEndpoint,
		apiToken:    apiToken,
		client:      &http.Client{},
		debug:       debug,
	}
}

func (s *NetBirdService) makeRequest(method, path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s", s.apiEndpoint, path)

	if s.debug {
		fmt.Printf("DEBUG: Making %s request to %s\n", method, url)
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Token %s", s.apiToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if s.debug {
		fmt.Printf("DEBUG: Request headers: %v\n", req.Header)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if s.debug {
		fmt.Printf("DEBUG: Response status: %d\n", resp.StatusCode)
		fmt.Printf("DEBUG: Response headers: %v\n", resp.Header)
		if resp.StatusCode >= 400 {
			fmt.Printf("DEBUG: Response body: %s\n", string(body))
		}
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (s *NetBirdService) Get(path string, result interface{}) error {
	body, err := s.makeRequest("GET", path)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, result)
}
