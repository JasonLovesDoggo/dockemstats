package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenResponse represents the auth token response
type TokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// Registry represents a Docker registry configuration
type Registry struct {
	Name        string
	AuthURL     string
	RegistryURL string
	Service     string
}

var registryConfigs = map[string]Registry{
	"dockerhub": {
		Name:        "Docker Hub",
		AuthURL:     "https://auth.docker.io/token",
		RegistryURL: "https://registry-1.docker.io",
		Service:     "registry.docker.io",
	},
	"ghcr": {
		Name:        "GitHub Container Registry",
		AuthURL:     "https://ghcr.io/token",
		RegistryURL: "https://ghcr.io",
		Service:     "ghcr.io",
	},
}

func getToken(registry Registry, repository string) (string, error) {
	url := fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", registry.AuthURL, registry.Service, repository)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token, status: %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	// Some registries use token, others use access_token
	if tokenResp.Token != "" {
		return tokenResp.Token, nil
	}
	return tokenResp.AccessToken, nil
}

func simulateManifestPull(registry Registry, imageSpec string, connID int, wg *sync.WaitGroup) {
	defer wg.Done()

	// Parse image name and tag
	repository := imageSpec
	tag := "latest"

	if strings.Contains(imageSpec, ":") {
		parts := strings.Split(imageSpec, ":")
		repository = parts[0]
		tag = parts[1]
	}

	// For Docker Hub, if it doesn't already have a path prefix (like library/ or username/)
	if registry.Name == "Docker Hub" && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	// Remove registry prefix if it exists in the repository name for GHCR
	if registry.Name == "GitHub Container Registry" && strings.HasPrefix(repository, "ghcr.io/") {
		repository = strings.TrimPrefix(repository, "ghcr.io/")
	}

	fmt.Printf("Connection %d: Getting token for %s from %s\n", connID, repository, registry.Name)

	token, err := getToken(registry, repository)
	if err != nil {
		fmt.Printf("Connection %d: Failed to get token: %v\n", connID, err)
		return
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registry.RegistryURL, repository, tag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Connection %d: Error creating request: %v\n", connID, err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	fmt.Printf("Connection %d: Requesting manifest for %s:%s from %s\n", connID, repository, tag, registry.Name)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Connection %d: Error requesting manifest: %v\n", connID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Connection %d: Successfully requested manifest (Status: %d)\n", connID, resp.StatusCode)
	} else {
		fmt.Printf("Connection %d: Failed to request manifest (Status: %d)\n", connID, resp.StatusCode)
	}
}

func main() {
	var (
		imageName    = flag.String("image", "", "Docker image name (e.g., nginx:latest or ghcr.io/username/repo:tag)")
		numPulls     = flag.Int("pulls", 1, "Number of parallel pulls to simulate")
		registryName = flag.String("registry", "dockerhub", "Registry to use (dockerhub, ghcr)")
		delay        = flag.Int("delay", 100, "Delay between requests in milliseconds")
	)
	flag.Parse()

	if *imageName == "" {
		fmt.Println("Error: image name is required")
		flag.Usage()
		return
	}

	registry, ok := registryConfigs[*registryName]
	if !ok {
		fmt.Printf("Error: unsupported registry %s. Supported registries: %v\n",
			*registryName, getRegistryKeys())
		return
	}

	fmt.Printf("Starting %d parallel manifest requests for %s from %s\n",
		*numPulls, *imageName, registry.Name)

	var wg sync.WaitGroup
	wg.Add(*numPulls)

	for i := 0; i < *numPulls; i++ {
		go simulateManifestPull(registry, *imageName, i+1, &wg)
		// Add configurable delay between starting connections
		time.Sleep(time.Duration(*delay) * time.Millisecond)
	}

	wg.Wait()
	fmt.Println("All manifest requests completed!")
}

func getRegistryKeys() []string {
	keys := make([]string, 0, len(registryConfigs))
	for k := range registryConfigs {
		keys = append(keys, k)
	}
	return keys
}
