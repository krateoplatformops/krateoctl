package kube

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func RestConfig() (*rest.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	fn := filepath.Join(home, ".kube", "config")
	if pt := os.Getenv("KUBECONFIG"); len(pt) > 0 {
		fn = filepath.Join(home, pt)
	}

	return clientcmd.BuildConfigFromFlags("", fn)
}
