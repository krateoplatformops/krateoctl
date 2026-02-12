//go:build integration
// +build integration

package steps

import (
	"context"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/support/kind"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
)

var (
	objTestEnv     env.Environment
	objClusterName string
)

const (
	objNamespace = "object-test-system"
)

func TestMain(m *testing.M) {
	objClusterName = "object-test"
	objTestEnv = env.New()

	objTestEnv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), objClusterName),
		createObjectNamespace(objNamespace),
		setupObjectTestData,
	).Finish(
		envfuncs.DestroyCluster(objClusterName),
	)

	os.Exit(objTestEnv.Run(m))
}

func createObjectNamespace(ns string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, err := resources.New(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, err
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		}

		return ctx, r.Create(ctx, namespace)
	}
}

func setupObjectTestData(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	// Setup any prerequisite data for object tests
	return ctx, nil
}

func TestObjectStepHandlerE2E(t *testing.T) {
	feature := features.New("Object Step Handler E2E Tests").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return ctx
		}).
		Assess("Create ConfigMap", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Create)

			objJSON := `{
                "apiVersion": "v1",
                "kind": "ConfigMap",
                "metadata": {
                    "name": "test-configmap-create",
                    "namespace": "object-test-system"
                },
                "data": {
                    "key1": "value1",
                    "key2": "value2"
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-create-cm", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Operation != "apply" {
				t.Errorf("Expected 'apply', got '%s'", result.Operation)
			}

			if result.Kind != "ConfigMap" {
				t.Errorf("Expected 'ConfigMap', got '%s'", result.Kind)
			}

			if result.Name != "test-configmap-create" {
				t.Errorf("Expected name 'test-configmap-create', got '%s'", result.Name)
			}

			// Verify the ConfigMap was actually created
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("Failed to create resources client: %v", err)
			}

			var cm corev1.ConfigMap
			if err := r.Get(ctx, "test-configmap-create", objNamespace, &cm); err != nil {
				t.Fatalf("Failed to get created ConfigMap: %v", err)
			}

			if cm.Data["key1"] != "value1" {
				t.Errorf("Expected key1='value1', got '%s'", cm.Data["key1"])
			}

			if cm.Data["key2"] != "value2" {
				t.Errorf("Expected key2='value2', got '%s'", cm.Data["key2"])
			}

			t.Logf("ConfigMap creation test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Assess("Create Secret", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Create)

			objJSON := `{
                "apiVersion": "v1",
                "kind": "Secret",
                "metadata": {
                    "name": "test-secret-create",
                    "namespace": "object-test-system"
                },
                "type": "Opaque",
                "stringData": {
                    "username": "admin",
                    "password": "secret123"
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-create-secret", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Kind != "Secret" {
				t.Errorf("Expected 'Secret', got '%s'", result.Kind)
			}

			// Verify the Secret was actually created
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("Failed to create resources client: %v", err)
			}

			var secret corev1.Secret
			if err := r.Get(ctx, "test-secret-create", objNamespace, &secret); err != nil {
				t.Fatalf("Failed to get created Secret: %v", err)
			}

			if string(secret.Data["username"]) != "admin" {
				t.Errorf("Expected username='admin', got '%s'", string(secret.Data["username"]))
			}

			if string(secret.Data["password"]) != "secret123" {
				t.Errorf("Expected password='secret123', got '%s'", string(secret.Data["password"]))
			}

			t.Logf("Secret creation test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Assess("Update ConfigMap", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Update)

			objJSON := `{
                "apiVersion": "v1",
                "kind": "ConfigMap",
                "metadata": {
                    "name": "test-configmap-create",
                    "namespace": "object-test-system"
                },
                "data": {
                    "key1": "updated-value1",
                    "key3": "new-value3"
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-update-cm", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Operation != "apply" {
				t.Errorf("Expected 'apply', got '%s'", result.Operation)
			}

			// Wait a bit for the update to propagate
			time.Sleep(1 * time.Second)

			// Verify the ConfigMap was updated
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("Failed to create resources client: %v", err)
			}

			var cm corev1.ConfigMap
			if err := r.Get(ctx, "test-configmap-create", objNamespace, &cm); err != nil {
				t.Fatalf("Failed to get updated ConfigMap: %v", err)
			}

			if cm.Data["key1"] != "updated-value1" {
				t.Errorf("Expected key1='updated-value1', got '%s'", cm.Data["key1"])
			}

			if cm.Data["key3"] != "new-value3" {
				t.Errorf("Expected key3='new-value3', got '%s'", cm.Data["key3"])
			}

			t.Logf("ConfigMap update test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Assess("Delete ConfigMap", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Delete)

			objJSON := `{
                "apiVersion": "v1",
                "kind": "ConfigMap",
                "metadata": {
                    "name": "test-configmap-create",
                    "namespace": "object-test-system"
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-delete-cm", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Operation != "delete" {
				t.Errorf("Expected 'delete', got '%s'", result.Operation)
			}

			time.Sleep(2 * time.Second) // Wait for deletion to propagate

			// Verify the ConfigMap was deleted
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("Failed to create resources client: %v", err)
			}

			var cm corev1.ConfigMap
			if err := r.Get(ctx, "test-configmap-create", objNamespace, &cm); err == nil {
				t.Error("ConfigMap should have been deleted but still exists")
			}

			t.Logf("ConfigMap deletion test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Assess("Create with default namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Create)

			// No namespace specified in metadata, should use handler's default
			objJSON := `{
                "apiVersion": "v1",
                "kind": "ConfigMap",
                "metadata": {
                    "name": "test-configmap-default-ns"
                },
                "data": {
                    "test": "default-namespace-value"
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-default-ns-cm", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Namespace != objNamespace {
				t.Errorf("Expected namespace '%s', got '%s'", objNamespace, result.Namespace)
			}

			// Verify the ConfigMap was created in the correct namespace
			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fatalf("Failed to create resources client: %v", err)
			}

			var cm corev1.ConfigMap
			if err := r.Get(ctx, "test-configmap-default-ns", objNamespace, &cm); err != nil {
				t.Fatalf("Failed to get ConfigMap in default namespace: %v", err)
			}

			if cm.Data["test"] != "default-namespace-value" {
				t.Errorf("Expected data.test='default-namespace-value', got '%s'", cm.Data["test"])
			}

			t.Logf("Default namespace test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Assess("Create with BodyFields merging", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			handler, err := createObjectHandler(cfg)
			if err != nil {
				t.Fatalf("Failed to create object handler: %v", err)
			}

			handler.Namespace(objNamespace)
			handler.Op(steps.Create)

			// Test object with multiple fields that should be captured in BodyFields
			objJSON := `{
                "apiVersion": "apps/v1",
                "kind": "Deployment",
                "metadata": {
                    "name": "test-deployment",
                    "namespace": "object-test-system"
                },
                "spec": {
                    "replicas": 3,
                    "selector": {
                        "matchLabels": {
                            "app": "test"
                        }
                    },
                    "template": {
                        "metadata": {
                            "labels": {
                                "app": "test"
                            }
                        },
                        "spec": {
                            "containers": [
                                {
                                    "name": "test-container",
                                    "image": "nginx:latest",
                                    "ports": [
                                        {
                                            "containerPort": 80
                                        }
                                    ]
                                }
                            ]
                        }
                    }
                }
            }`

			ext := &runtime.RawExtension{Raw: []byte(objJSON)}
			result, err := handler.Handle(ctx, "test-deployment", ext)

			if err != nil {
				t.Fatalf("Handler failed: %v", err)
			}

			if result.Kind != "Deployment" {
				t.Errorf("Expected 'Deployment', got '%s'", result.Kind)
			}

			if result.Name != "test-deployment" {
				t.Errorf("Expected name 'test-deployment', got '%s'", result.Name)
			}

			t.Logf("BodyFields merging test passed: %s/%s", result.Namespace, result.Name)
			return ctx
		}).
		Feature()

	objTestEnv.Test(t, feature)
}

// Helper functions
func createObjectHandler(cfg *envconf.Config) (*objStepHandler, error) {
	applier, err := applier.NewApplier(cfg.Client().RESTConfig())
	if err != nil {
		return nil, err
	}

	deletor, err := deletor.NewDeletor(cfg.Client().RESTConfig())
	if err != nil {
		return nil, err
	}

	env := cache.New[string, string]()
	zl := zap.New(zap.UseDevMode(true))
	log := logging.NewLogrLogger(zl.WithName("object-test"))

	handler := ObjectHandler(applier, deletor, env, log)
	return handler.(*objStepHandler), nil
}
