package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

// Secret names and keys used in the installation process
const (
	JWTSecretName          = "jwt-sign-key"
	KrateoDbSecretName     = "krateo-db"
	KrateoDbUserSecretName = "krateo-db-user"
)

// GenerateRandomKey creates a random base64-encoded key of specified length
func GenerateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// CreateJWTSigningSecret creates the JWT signing secret with a random key
func CreateJWTSigningSecret(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	jwtKey, err := GenerateRandomKey(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT key: %w", err)
	}

	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.SetName(JWTSecretName)
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"JWT_SIGN_KEY": jwtKey,
	}

	return secret, nil
}

// CreateKrateoDbSecret creates the secret used by resources-stack and events-stack components
func CreateKrateoDbSecret(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	dbPass, err := GenerateRandomKey(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate database password: %w", err)
	}
	return createKrateoDbSecretWithPassword(namespace, dbPass), nil
}

// createKrateoDbSecretWithPassword creates the secret with a provided password
func createKrateoDbSecretWithPassword(namespace, password string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.SetName(KrateoDbSecretName)
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"DB_USER": KrateoDbUserSecretName,
		"DB_PASS": password,
	}

	return secret
}

// CreateKrateoDbUserSecret creates the secret for the CNPG cluster
func CreateKrateoDbUserSecret(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	password, err := GenerateRandomKey(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user password: %w", err)
	}
	return createKrateoDbUserSecretWithPassword(namespace, password), nil
}

// createKrateoDbUserSecretWithPassword creates the secret with a provided password
func createKrateoDbUserSecretWithPassword(namespace, password string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.SetName(KrateoDbUserSecretName)
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"username": KrateoDbUserSecretName,
		"password": password,
	}

	return secret
}

// InitializeSecrets creates all required secrets in the cluster
func InitializeSecrets(ctx context.Context, cfg *rest.Config, namespace string) error {
	appl, err := applier.NewApplier(cfg)
	if err != nil {
		return fmt.Errorf("failed to create applier: %w", err)
	}

	// Generate a shared password for both db and db-user secrets to ensure they are consistent
	sharedPassword, err := GenerateRandomKey(16)
	if err != nil {
		return fmt.Errorf("failed to generate shared password: %w", err)
	}

	secrets := []*unstructured.Unstructured{}

	jwtSecret, err := CreateJWTSigningSecret(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to create JWT secret: %w", err)
	}
	secrets = append(secrets, jwtSecret)

	// Use the shared password for db and db-user secrets to ensure they are consistent
	krateoDbSecret := createKrateoDbSecretWithPassword(namespace, sharedPassword)
	secrets = append(secrets, krateoDbSecret)

	krateoDbUserSecret := createKrateoDbUserSecretWithPassword(namespace, sharedPassword)
	secrets = append(secrets, krateoDbUserSecret)

	for _, secret := range secrets {
		gvk := secret.GetObjectKind().GroupVersionKind()
		opts := applier.ApplyOptions{
			GVK:       gvk,
			Namespace: secret.GetNamespace(),
			Name:      secret.GetName(),
		}
		if err := appl.Apply(ctx, secret.Object, opts); err != nil {
			return fmt.Errorf("failed to apply secret %s: %w", secret.GetName(), err)
		}
	}

	return nil
}
