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
	secret.SetName("jwt-sign-key")
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"JWT_SIGN_KEY": jwtKey,
	}

	return secret, nil
}

// CreateEventStackDBSecret creates the events-stack-db secret for CNPG
func CreateEventStackDBSecret(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	dbPass, err := GenerateRandomKey(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate database password: %w", err)
	}
	return createEventStackDBSecretWithPassword(namespace, dbPass), nil
}

// createEventStackDBSecretWithPassword creates the events-stack-db secret with a provided password
func createEventStackDBSecretWithPassword(namespace, password string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.SetName("events-stack-db")
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"DB_USER": "events_user",
		"DB_PASS": password,
	}

	return secret
}

// CreateEventsUserSecret creates the events-user-secret for CNPG cluster
func CreateEventsUserSecret(ctx context.Context, namespace string) (*unstructured.Unstructured, error) {
	password, err := GenerateRandomKey(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user password: %w", err)
	}
	return createEventsUserSecretWithPassword(namespace, password), nil
}

// createEventsUserSecretWithPassword creates the events-user-secret with a provided password
func createEventsUserSecretWithPassword(namespace, password string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	secret.SetName("events-user-secret")
	secret.SetNamespace(namespace)

	secret.Object["type"] = "Opaque"
	secret.Object["stringData"] = map[string]interface{}{
		"username": "events_user",
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

	// Generate a shared password for both events-stack-db and events-user-secret
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

	// Use the shared password for events-stack-db
	stackDBSecret := createEventStackDBSecretWithPassword(namespace, sharedPassword)
	secrets = append(secrets, stackDBSecret)

	// Use the same shared password for events-user-secret
	userSecret := createEventsUserSecretWithPassword(namespace, sharedPassword)
	secrets = append(secrets, userSecret)

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
