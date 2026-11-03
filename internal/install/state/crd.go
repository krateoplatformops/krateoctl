package state

import (
	"context"
	"fmt"

	_ "embed"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

//go:embed manifest/installation.crd.yaml
var installationCRD []byte

// EnsureCRD installs the Installation CRD if it is not already present in the cluster.
func EnsureCRD(ctx context.Context, cfg *rest.Config) error {
	client, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("build apiextensions client: %w", err)
	}

	var crd apiextv1.CustomResourceDefinition
	if err := yaml.Unmarshal(installationCRD, &crd); err != nil {
		return fmt.Errorf("parse embedded installation CRD: %w", err)
	}

	_, err = client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crd, metav1.CreateOptions{})
	}

	return err
}
