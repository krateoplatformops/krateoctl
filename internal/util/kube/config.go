package kube

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// RestConfig returns a REST config for communicating with Kubernetes API.
func RestConfig() (*rest.Config, error) {
	cfg, err := ClientConfig()
	if err != nil {
		return nil, err
	}

	return cfg.ClientConfig()
}

// ClientConfig loads the kubeconfig the way kubectl does: the files listed in
// KUBECONFIG when set, ~/.kube/config otherwise. Without a kubeconfig it falls
// back to in-cluster ServiceAccount credentials.
func ClientConfig() (clientcmd.ClientConfig, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
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
