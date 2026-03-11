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

func RestConfig() (*rest.Config, error) {
	fn, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}

	return clientcmd.BuildConfigFromFlags("", fn)
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
