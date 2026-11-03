package kube

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func kubeconfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	fn := filepath.Join(home, ".kube", "config")
	if pt := os.Getenv("KUBECONFIG"); len(pt) > 0 {
		fn = filepath.Join(home, pt)
	}

	return fn, nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RestConfig returns a REST config for communicating with Kubernetes API.
// It attempts to load from kubeconfig first, then falls back to in-cluster
// ServiceAccount credentials if kubeconfig is not available.
func RestConfig() (*rest.Config, error) {
	fn, err := kubeconfigPath()
	if err == nil && fileExists(fn) {
		// Kubeconfig exists, use it
		return clientcmd.BuildConfigFromFlags("", fn)
	}

	// Fallback to in-cluster ServiceAccount authentication
	return rest.InClusterConfig()
}

func ClientConfig() (clientcmd.ClientConfig, error) {
	fn, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}

	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: fn}
	overrides := &clientcmd.ConfigOverrides{}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides), nil
}

func DefaultNamespace() (string, error) {
	cfg, err := ClientConfig()
	if err != nil {
		return "", err
	}

	ns, _, err := cfg.Namespace()
	if err != nil {
		return "", err
	}

	if ns == "" {
		return "default", nil
	}

	return ns, nil
}
