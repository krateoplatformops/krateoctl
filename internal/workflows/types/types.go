package types

import (
	"encoding/json"
	"strconv"
	"time"

	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"

	rtv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"
	"github.com/twmb/murmur3"
)

type Data struct {
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	AsString *bool  `json:"asString,omitempty"`
}

type ObjectMeta struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   rtv1.Reference `json:"metadata"`
}

type ValueFromSource struct {
	ObjectMeta `json:",inline"`
	Selector   string `json:"selector,omitempty"`
}

type Var struct {
	Data      `json:",inline"`
	ValueFrom *ValueFromSource `json:"valueFrom,omitempty"`
}

type Credentials struct {
	Username    string                 `json:"username"`
	PasswordRef rtv1.SecretKeySelector `json:"passwordRef"`
}

type ChartSpec struct {
	// Repository: Helm repository URL, required if ChartSpec.URL not set
	URL string `json:"url,omitempty"`
	// Name of Helm chart, required if ChartSpec.URL not set
	Repo string `json:"repo,omitempty"`
	// Version of Helm chart, late initialized with latest version if not set
	Version string `json:"version,omitempty"`

	// ReleaseName is the name of the release. If not set, Repo will be used or it will be derived from the URL
	ReleaseName string `json:"releaseName,omitempty"`

	// MaxHistory is the maximum number of helm releases to keep in history
	MaxHistory *int `json:"maxHistory,omitempty"`

	// Namespace to install the release into.
	Namespace string `json:"namespace"`

	// Wait for the release to become ready.
	Wait bool `json:"wait,omitempty"`

	// Timeout is the time to wait for any individual kubernetes operation (like Jobs for hooks) to complete. Defaults to 5m.
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Deprecated: use Timeout instead. This is the time to wait for the release to become ready. Only applies if wait is also set. Defaults to 5m.
	WaitTimeout *metav1.Duration `json:"waitTimeout,omitempty"`

	// Values defines the Helm values
	Values map[string]any `json:"values,omitempty"`

	// InsecureSkipTLSVerify skips tls certificate checks for the chart download
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`
}

// SetDefaults applies default values to optional fields.
// IMPORTANT: You must call this method after unmarshaling the JSON.
func (c *ChartSpec) SetDefaults() {
	// Handle Timeout Default (5 minutes)
	if c.Timeout == nil {
		if c.WaitTimeout != nil {
			// Backward compatibility: use WaitTimeout if set
			c.Timeout = c.WaitTimeout
		} else {
			// Default to 5 minutes
			c.Timeout = &metav1.Duration{Duration: 5 * time.Minute}
		}
	}

	if c.MaxHistory == nil {
		defaultMaxHistory := 10
		c.MaxHistory = &defaultMaxHistory
	}
}

type ChartObservation struct {
	State    release.Status `json:"state,omitempty"`
	Revision int            `json:"revision,omitempty"`
}

type Object struct {
	ObjectMeta `json:",inline"`
	BodyFields map[string]any `json:"-"`
}

// UnmarshalJSON captures all fields not in ObjectMeta into BodyFields
func (o *Object) UnmarshalJSON(data []byte) error {
	// First unmarshal into a raw map to capture all fields
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Unmarshal ObjectMeta fields
	var om ObjectMeta
	if err := json.Unmarshal(data, &om); err != nil {
		return err
	}
	o.ObjectMeta = om

	// Capture all fields except apiVersion, kind, and metadata into BodyFields
	o.BodyFields = make(map[string]any)
	for k, v := range raw {
		if k != "apiVersion" && k != "kind" && k != "metadata" {
			o.BodyFields[k] = v
		}
	}

	return nil
}

type StepType string

const (
	TypeObject StepType = "object"
	TypeChart  StepType = "chart"
	TypeVar    StepType = "var"
)

type Step struct {
	ID   string                `json:"id"`
	Type StepType              `json:"type"`
	With *runtime.RawExtension `json:"with"`
	Skip bool                  `json:"skip,omitempty"`
}

func (s *Step) Digest() string {
	if s.With == nil || len(s.With.Raw) == 0 {
		return ""
	}

	hasher := murmur3.New64()
	hasher.Write(s.With.Raw)

	return strconv.FormatUint(hasher.Sum64(), 16)
}

type Workflow struct {
	Steps []*Step `json:"steps,omitempty"`
}
