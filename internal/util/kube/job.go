package kube

import (
	"context"
	"fmt"
	"time"

	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// JobWaiter waits for Kubernetes Jobs to complete.
type JobWaiter struct {
	getter  *getter.Getter
	timeout time.Duration
}

type jobStatus int

const (
	jobStatusRunning jobStatus = iota
	jobStatusSucceeded
	jobStatusFailed
)

// NewJobWaiter creates a new JobWaiter with a default 5-minute timeout.
func NewJobWaiter(g *getter.Getter) *JobWaiter {
	return &JobWaiter{
		getter:  g,
		timeout: 5 * time.Minute,
	}
}

// WithTimeout sets a custom timeout for job completion.
func (jw *JobWaiter) WithTimeout(timeout time.Duration) *JobWaiter {
	jw.timeout = timeout
	return jw
}

// Wait blocks until the given Job completes (succeeds or fails) or times out.
// Returns an error if the Job fails or the context/timeout is exceeded.
func (jw *JobWaiter) Wait(ctx context.Context, namespace, jobName string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, jw.timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for Job %s/%s to complete after %v", namespace, jobName, jw.timeout)
		case <-ticker.C:
			status, err := jw.checkStatus(timeoutCtx, namespace, jobName)
			if err != nil {
				// Log but continue polling
				continue
			}

			if status == jobStatusFailed {
				return fmt.Errorf("Job %s/%s failed", namespace, jobName)
			}

			if status == jobStatusSucceeded {
				return nil
			}
		}
	}
}

// checkStatus fetches the Job and checks if it has succeeded or failed.
func (jw *JobWaiter) checkStatus(ctx context.Context, namespace, jobName string) (jobStatus, error) {
	opts := getter.GetOptions{
		GVK:       schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		Namespace: namespace,
		Name:      jobName,
	}

	job, err := jw.getter.Get(ctx, opts)
	if err != nil {
		return jobStatusRunning, err
	}

	return parseJobConditions(job)
}

// parseJobConditions examines a Job's status conditions and returns its state.
func parseJobConditions(job *unstructured.Unstructured) (jobStatus, error) {
	// Extract status from Job object
	status, ok := job.Object["status"].(map[string]interface{})
	if !ok {
		return jobStatusRunning, nil // No status yet
	}

	// Check conditions array
	conditions, ok := status["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		return jobStatusRunning, nil // No conditions yet, still running
	}

	// Evaluate conditions
	for _, cond := range conditions {
		condition, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, okType := condition["type"].(string)
		condStatus, okStatus := condition["status"].(string)

		if !okType || !okStatus {
			continue
		}

		// Job succeeded
		if condType == "Complete" && condStatus == "True" {
			return jobStatusSucceeded, nil
		}

		// Job failed
		if condType == "Failed" && condStatus == "True" {
			return jobStatusFailed, nil
		}
	}

	// Still running
	return jobStatusRunning, nil
}
