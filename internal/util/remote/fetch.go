package remote

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultRepository is the default GitHub repository for Krateo releases
	DefaultRepository = "https://github.com/krateoplatformops/releases"
	// DefaultTimeout for HTTP requests
	DefaultTimeout = 30 * time.Second
)

// FetchOptions configures how remote files are fetched.
type FetchOptions struct {
	// Repository is the GitHub repository URL (e.g., https://github.com/owner/repo)
	Repository string
	// Version is the git tag/version to fetch from
	Version string
	// Filename is the name of the file to fetch (e.g., krateo.yaml)
	Filename string
	// Timeout for HTTP requests
	Timeout time.Duration
}

// Fetcher handles fetching files from remote repositories.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new remote file fetcher.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// FetchFile downloads a file from a GitHub repository at a specific tag/version.
// Returns the file contents as bytes.
func (f *Fetcher) FetchFile(opts FetchOptions) ([]byte, error) {
	if opts.Repository == "" {
		return nil, fmt.Errorf("repository URL is required")
	}
	if opts.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if opts.Filename == "" {
		return nil, fmt.Errorf("filename is required")
	}

	// Construct the raw GitHub URL
	rawURL, err := constructRawURL(opts.Repository, opts.Version, opts.Filename)
	if err != nil {
		return nil, fmt.Errorf("failed to construct URL: %w", err)
	}

	// Fetch the file
	resp, err := f.client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}

	// Read the response body
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return content, nil
}

// constructRawURL builds the raw GitHub URL for a file at a specific tag.
// Example: https://raw.githubusercontent.com/krateoplatformops/releases/v1.0.0/krateo.yaml
func constructRawURL(repoURL, version, filename string) (string, error) {
	// Parse the repository URL
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Extract owner and repo from the path
	// Path format: /owner/repo or /owner/repo.git
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		return "", fmt.Errorf("invalid repository URL format: expected github.com/owner/repo")
	}

	owner := parts[0]
	repo := parts[1]

	// Construct the raw URL
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repo, version, filename)

	return rawURL, nil
}

// IsRemoteSource checks if a config path should be fetched remotely
// based on whether a version is specified.
func IsRemoteSource(version string) bool {
	return version != ""
}
