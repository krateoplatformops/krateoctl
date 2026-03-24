package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/util/remote"
	"gopkg.in/yaml.v3"
)

// LoadOptions configures how configuration is loaded.
type LoadOptions struct {
	// ConfigPath is the path to the main krateo.yaml (used for local mode)
	ConfigPath string
	// UserOverridesPath is the path to krateo-overrides.yaml (optional)
	UserOverridesPath string
	// Profile is the optional name of a profile to apply from overrides
	Profile string
	// Repository is the GitHub repository URL to fetch config from (remote mode)
	Repository string
	// Version is the git tag/version to fetch from the repository (remote mode)
	Version string
	// InstallationType is the deployment type (nodeport, loadbalancer, ingress)
	// Used to select type-specific config files
	InstallationType string
}

// Loader handles loading configuration from files.
type Loader struct {
	opts LoadOptions
}

// NewLoader creates a new configuration loader.
func NewLoader(opts LoadOptions) *Loader {
	return &Loader{opts: opts}
}

// Load reads and parses configuration from krateo.yaml and optional overrides.
// Returns a map[string]any representing the merged configuration.
func (l *Loader) Load() (map[string]any, error) {
	// Check if we're in remote mode (version specified)
	if remote.IsRemoteSource(l.opts.Version) {
		return l.loadRemote()
	}

	// Local mode: Load main config file from filesystem
	// Try type-specific file first (e.g., krateo.nodeport.yaml), then fallback to generic krateo.yaml
	config, err := l.loadConfigWithType(l.opts.ConfigPath, l.opts.InstallationType)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// If no overrides path is configured, we're done.
	if l.opts.UserOverridesPath == "" {
		return config, nil
	}

	// Load base overrides file if it exists. It's optional, but its directory
	// is also used as the anchor for profile-specific override files
	// (krateo-overrides.<profile>.yaml).
	baseOverrides := make(map[string]any)
	if fi, err := os.Stat(l.opts.UserOverridesPath); err == nil && !fi.IsDir() {
		baseOverrides, err = l.loadFile(l.opts.UserOverridesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load overrides from %s: %w", l.opts.UserOverridesPath, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat overrides file %s: %w", l.opts.UserOverridesPath, err)
	}

	// Determine effective profile list: CLI flag wins, otherwise fall back to
	// the top-level "profile" key inside krateo-overrides.yaml if present.
	profileStr := strings.TrimSpace(l.opts.Profile)
	if profileStr == "" {
		if v, ok := baseOverrides["profile"].(string); ok {
			profileStr = v
		}
	}
	profiles := parseProfiles(profileStr)

	// Collect all profile-derived overrides (from both separate
	// krateo-overrides.<profile>.yaml files and the in-file "profiles" map)
	// before finally applying the base krateo-overrides.yaml. This ensures
	// that krateo-overrides.yaml is applied *after* all profiles, so that
	// top-level overrides win over any profile.
	profileOverrides := make(map[string]any)
	foundProfiles := make(map[string]bool)

	if len(profiles) > 0 {
		dir := filepath.Dir(l.opts.UserOverridesPath)
		base := filepath.Base(l.opts.UserOverridesPath)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)

		var profilesMap map[string]any
		if profilesRaw, ok := baseOverrides["profiles"]; ok {
			var ok2 bool
			profilesMap, ok2 = profilesRaw.(map[string]any)
			if !ok2 {
				return nil, fmt.Errorf("profiles must be a mapping, got %T", profilesRaw)
			}
		}

		// 1) Profile-specific override files: krateo-overrides.<profile>.yaml
		for _, p := range profiles {
			profFile := fmt.Sprintf("%s.%s%s", name, p, ext)
			profPath := filepath.Join(dir, profFile)

			if fi, err := os.Stat(profPath); err == nil && !fi.IsDir() {
				profData, err := l.loadFile(profPath)
				if err != nil {
					return nil, fmt.Errorf("failed to load profile overrides from %s: %w", profPath, err)
				}
				profileOverrides = mergeConfigs(profileOverrides, profData)
				foundProfiles[p] = true
			} else if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to stat profile overrides file %s: %w", profPath, err)
			}
		}

		// 2) In-file profiles defined inside base overrides (if any)
		if profilesMap != nil {
			for _, p := range profiles {
				if entryRaw, ok := profilesMap[p]; ok {
					entryMap, ok := entryRaw.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("profile %q must be a mapping, got %T", p, entryRaw)
					}
					profileOverrides = mergeConfigs(profileOverrides, entryMap)
					foundProfiles[p] = true
				}
			}
		}

		// 3) Validate that all requested profiles were found
		for _, p := range profiles {
			if !foundProfiles[p] {
				return nil, l.profileNotFoundError(p)
			}
		}
	}

	// Remove profile metadata from the base overrides so it doesn't leak into
	// the final configuration.
	delete(baseOverrides, "profiles")
	delete(baseOverrides, "profile")

	// Merge order:
	//   base config <- profile overrides <- base krateo-overrides.yaml
	// so that krateo-overrides.yaml always has the last word.
	if len(profileOverrides) > 0 {
		config = mergeConfigs(config, profileOverrides)
	}
	if len(baseOverrides) > 0 {
		config = mergeConfigs(config, baseOverrides)
	}

	return config, nil
}

// loadRemote fetches configuration from a remote GitHub repository.
func (l *Loader) loadRemote() (map[string]any, error) {
	repo := l.opts.Repository
	if repo == "" {
		repo = remote.DefaultRepository
	}

	// Fetch the main config file from the remote repository
	// Try type-specific file first (e.g., krateo.nodeport.yaml), then fallback to generic krateo.yaml
	config, err := l.loadRemoteConfigWithType(repo, l.opts.Version, l.opts.InstallationType)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s@%s: %w", repo, l.opts.Version, err)
	}

	// Try to fetch overrides file (optional), fallback to local if not found remotely
	baseOverrides, err := l.loadRemoteFile(repo, l.opts.Version, "krateo-overrides.yaml")
	if err != nil {
		// Try to load from local filesystem as fallback
		baseOverrides = make(map[string]any)
		if l.opts.UserOverridesPath != "" {
			if fi, err := os.Stat(l.opts.UserOverridesPath); err == nil && !fi.IsDir() {
				baseOverrides, err = l.loadFile(l.opts.UserOverridesPath)
				if err != nil {
					return nil, fmt.Errorf("failed to load local overrides from %s: %w", l.opts.UserOverridesPath, err)
				}
			}
		}
	}

	// Determine effective profile list
	profileStr := strings.TrimSpace(l.opts.Profile)
	if profileStr == "" {
		if v, ok := baseOverrides["profile"].(string); ok {
			profileStr = v
		}
	}
	profiles := parseProfiles(profileStr)

	// Collect profile-specific overrides
	profileOverrides := make(map[string]any)
	foundProfiles := make(map[string]bool)

	if len(profiles) > 0 {
		var profilesMap map[string]any
		if profilesRaw, ok := baseOverrides["profiles"]; ok {
			var ok2 bool
			profilesMap, ok2 = profilesRaw.(map[string]any)
			if !ok2 {
				return nil, fmt.Errorf("profiles must be a mapping, got %T", profilesRaw)
			}
		}

		// Try to fetch profile-specific override files, fallback to local if not found remotely
		for _, p := range profiles {
			profFile := fmt.Sprintf("krateo-overrides.%s.yaml", p)

			// Try remote first
			profData, err := l.loadRemoteFile(repo, l.opts.Version, profFile)
			if err == nil {
				profileOverrides = mergeConfigs(profileOverrides, profData)
				foundProfiles[p] = true
				continue
			}

			// Fallback to local if UserOverridesPath is specified
			if l.opts.UserOverridesPath != "" {
				dir := filepath.Dir(l.opts.UserOverridesPath)
				localPath := filepath.Join(dir, profFile)

				if fi, err := os.Stat(localPath); err == nil && !fi.IsDir() {
					profData, err = l.loadFile(localPath)
					if err != nil {
						return nil, fmt.Errorf("failed to load local profile overrides from %s: %w", localPath, err)
					}
					profileOverrides = mergeConfigs(profileOverrides, profData)
					foundProfiles[p] = true
				}
			}
		}

		// Check for in-file profiles defined inside base overrides
		if profilesMap != nil {
			for _, p := range profiles {
				if entryRaw, ok := profilesMap[p]; ok {
					entryMap, ok := entryRaw.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("profile %q must be a mapping, got %T", p, entryRaw)
					}
					profileOverrides = mergeConfigs(profileOverrides, entryMap)
					foundProfiles[p] = true
				}
			}
		}

		// Validate that all requested profiles were found
		for _, p := range profiles {
			if !foundProfiles[p] {
				return nil, l.profileNotFoundError(p)
			}
		}
	}

	// Remove profile metadata
	delete(baseOverrides, "profiles")
	delete(baseOverrides, "profile")

	// Merge configurations
	if len(profileOverrides) > 0 {
		config = mergeConfigs(config, profileOverrides)
	}
	if len(baseOverrides) > 0 {
		config = mergeConfigs(config, baseOverrides)
	}

	return config, nil
}

// loadRemoteFile fetches a file from a remote repository and parses it as YAML.
func (l *Loader) loadRemoteFile(repo, version, filename string) (map[string]any, error) {
	fetcher := remote.NewFetcher()
	content, err := fetcher.FetchFile(remote.FetchOptions{
		Repository: repo,
		Version:    version,
		Filename:   filename,
	})
	if err != nil {
		return nil, err
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", filename, err)
	}

	return data, nil
}

// loadConfigWithType attempts to load type-specific config file first, then falls back to generic krateo.yaml
// For example, if installType is "nodeport", it tries krateo.nodeport.yaml first, then krateo.yaml
func (l *Loader) loadConfigWithType(basePath string, installType string) (map[string]any, error) {
	if basePath == "" {
		return make(map[string]any), nil
	}

	// Try type-specific variants first.
	for _, candidate := range installationTypeCandidates(installType) {
		typeSpecificPath := strings.TrimSuffix(basePath, filepath.Ext(basePath)) + "." + candidate + filepath.Ext(basePath)
		if data, err := l.loadFile(typeSpecificPath); err == nil {
			return data, nil
		}
	}

	// Fallback to generic krateo.yaml
	return l.loadFile(basePath)
}

// loadRemoteConfigWithType attempts to fetch type-specific config file first, then falls back to generic krateo.yaml
func (l *Loader) loadRemoteConfigWithType(repo, version, installType string) (map[string]any, error) {
	for _, candidate := range installationTypeCandidates(installType) {
		filename := "krateo." + candidate + ".yaml"
		if data, err := l.loadRemoteFile(repo, version, filename); err == nil {
			return data, nil
		}
	}

	// Fallback to generic krateo.yaml
	return l.loadRemoteFile(repo, version, "krateo.yaml")
}

func installationTypeCandidates(installType string) []string {
	raw := strings.ToLower(strings.TrimSpace(installType))
	if raw == "" {
		return nil
	}

	hasYAMLSuffix := strings.HasSuffix(raw, ".yaml")
	base := strings.TrimSuffix(raw, ".yaml")

	switch base {
	case "kind":
		return []string{"kind", "nodeport"}
	case "nodeport":
		if hasYAMLSuffix {
			return []string{"nodeport", "kind"}
		}
		// Preserve the existing nodeport alias behavior while also supporting nodeport.yaml.
		return []string{"kind", "nodeport"}
	case "loadbalancer", "ingress":
		return []string{base}
	default:
		return []string{base}
	}
}

// loadFile reads and parses a YAML file into a map.
func (l *Loader) loadFile(path string) (map[string]any, error) {
	if path == "" {
		return make(map[string]any), nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", path, err)
	}

	return data, nil
}

// mergeConfigs recursively merges override config into base config.
// Arrays are replaced (atomic strategy), objects are merged recursively.
func mergeConfigs(base, override map[string]any) map[string]any {
	for key, val := range override {
		if baseVal, exists := base[key]; exists {
			// Both are maps - merge recursively
			if baseMap, ok := baseVal.(map[string]any); ok {
				if overrideMap, ok := val.(map[string]any); ok {
					base[key] = mergeConfigs(baseMap, overrideMap)
					continue
				}
			}
		}
		// Replace for scalars, arrays, or type mismatches
		base[key] = val
	}
	return base
}

// parseProfiles splits a comma-separated profile string into a slice,
// trimming whitespace and ignoring empty entries. This allows callers to
// specify multiple profiles like "dev,debug" which will be applied in
// the given order.
func parseProfiles(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		res = append(res, p)
	}
	return res
}

// profileNotFoundError returns a descriptive error message explaining how to define a profile.
func (l *Loader) profileNotFoundError(profileName string) error {
	errMsg := fmt.Sprintf("profile %q not found\n", profileName)
	errMsg += "\nProfiles can be defined in two ways:\n"
	errMsg += "  1. As separate files: krateo-overrides." + profileName + ".yaml\n"
	errMsg += "  2. In-file under the 'profiles' section of krateo-overrides.yaml:\n"
	errMsg += "       profiles:\n"
	errMsg += "         " + profileName + ":\n"
	errMsg += "           components:\n"
	errMsg += "             # Override component configurations here\n"
	errMsg += "\nFor complete documentation on how to define and use profiles, run:\n"
	errMsg += "  krateoctl install plan --help\n"
	return fmt.Errorf("%s", errMsg)
}
