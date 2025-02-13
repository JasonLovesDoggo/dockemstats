package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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

// User agents list
var userAgents = []string{
	"docker/24.0.6",
	"docker/23.0.3",
	"docker/20.10.22",
	"docker/19.03.13",
	"containerd/1.6.19",
	"containerd/1.5.13",
	"podman/4.4.1",
	"buildkit/0.11.6",
}

// IP prefixes for fake client IPs
var ipPrefixes = []string{
	"192.168.", "10.0.", "172.16.", "176.32.",
	"52.94.", "34.198.", "13.107.", "104.16.",
}

func getRandomUserAgent() string {
	return userAgents[rand.Intn(len(userAgents))]
}

func getRandomIP() string {
	prefix := ipPrefixes[rand.Intn(len(ipPrefixes))]
	return fmt.Sprintf("%s%d.%d", prefix, rand.Intn(256), rand.Intn(256))
}

func getRandomHost() string {
	hosts := []string{
		"worker-node", "runner", "builder", "ci-agent",
		"deployment", "kubernetes", "docker-host", "jenkins-agent",
	}
	prefix := hosts[rand.Intn(len(hosts))]
	return fmt.Sprintf("%s-%d.example.com", prefix, rand.Intn(1000))
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

func simulateManifestPull(registry Registry, imageSpec string, connID int, wg *sync.WaitGroup, counter *int64, logCh chan<- string) {
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

	// Get auth token
	token, err := getToken(registry, repository)
	if err != nil {
		logCh <- fmt.Sprintf("Connection %d: Failed to get token: %v", connID, err)
		return
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registry.RegistryURL, repository, tag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logCh <- fmt.Sprintf("Connection %d: Error creating request: %v", connID, err)
		return
	}

	// Add realistic headers
	clientIP := getRandomIP()
	clientHost := getRandomHost()
	userAgent := getRandomUserAgent()

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Forwarded-For", clientIP)
	req.Header.Set("X-Real-IP", clientIP)
	req.Header.Set("Host", strings.TrimPrefix(registry.RegistryURL, "https://"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logCh <- fmt.Sprintf("Connection %d: Error requesting manifest: %v", connID, err)
		return
	}
	defer resp.Body.Close()

	if connID%50 == 0 {
		logCh <- fmt.Sprintf("Connection %d: Manifest request from %s (%s) using %s completed with status: %d",
			connID, clientHost, clientIP, userAgent, resp.StatusCode)
	}

	atomic.AddInt64(counter, 1)
}

func renderProgressBar(done int64, total int, width int) string {
	percentage := float64(done) / float64(total)
	completedWidth := int(float64(width) * percentage)

	bar := "["
	for i := 0; i < width; i++ {
		if i < completedWidth {
			bar += "="
		} else {
			bar += " "
		}
	}
	bar += "]"

	return fmt.Sprintf("%s %.1f%% (%d/%d)", bar, percentage*100, done, total)
}

func main() {
	var (
		imageName    = flag.String("image", "", "Docker image name (e.g., nginx:latest or ghcr.io/username/repo:tag)")
		numPulls     = flag.Int("pulls", 1, "Number of pulls to simulate")
		registryName = flag.String("registry", "dockerhub", "Registry to use (dockerhub, ghcr)")
		delay        = flag.Int("delay", 50, "Delay between requests in milliseconds")
		concurrent   = flag.Int("concurrent", 5, "Number of concurrent requests")
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

	fmt.Printf("Starting %d manifest requests for %s from %s\n",
		*numPulls, *imageName, registry.Name)

	semaphore := make(chan struct{}, *concurrent)
	var wg sync.WaitGroup
	var counter int64 = 0
	startTime := time.Now()

	// Create a channel for logging
	logCh := make(chan string, *concurrent)

	// Start logger goroutine
	go func() {
		for msg := range logCh {
			fmt.Println("\r" + msg)
			fmt.Print("\r" + renderProgressBar(atomic.LoadInt64(&counter), *numPulls, 50))
		}
	}()

	// Progress bar goroutine
	ticker := time.NewTicker(time.Duration(*delay) * time.Millisecond)
	go func() {
		for range ticker.C {
			if atomic.LoadInt64(&counter) >= int64(*numPulls) {
				return
			}
			fmt.Print("\r" + renderProgressBar(atomic.LoadInt64(&counter), *numPulls, 50))
		}
	}()

	for i := 0; i < *numPulls; i++ {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(id int) {
			simulateManifestPull(registry, *imageName, id, &wg, &counter, logCh)
			<-semaphore
		}(i + 1)

		time.Sleep(time.Duration(*delay) * time.Millisecond)
	}

	wg.Wait()
	time.Sleep(time.Duration(*delay) * time.Millisecond) // Wait for the last log messages
	ticker.Stop()
	close(logCh)

	duration := time.Since(startTime)
	rate := float64(*numPulls) / duration.Seconds()

	fmt.Printf("\nAll manifest requests completed!\n")
	fmt.Printf("Time taken: %s\n", duration)
	fmt.Printf("Average rate: %.1f requests/second\n", rate)
}

func getRegistryKeys() []string {
	keys := make([]string, 0, len(registryConfigs))
	for k := range registryConfigs {
		keys = append(keys, k)
	}
	return keys
}
