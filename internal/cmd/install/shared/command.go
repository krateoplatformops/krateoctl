package shared

import (
	"io"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"k8s.io/client-go/rest"
)

func DebugEnabled(flagValue bool) bool {
	return flagValue
}

func NewLogger(writer io.Writer, debug bool) *ui.Logger {
	logLevel := ui.LevelInfo
	if debug {
		logLevel = ui.LevelDebug
	}
	return ui.NewLogger(writer, logLevel)
}

type LoadOptionsInput struct {
	ConfigFile       string
	Namespace        string
	Profile          string
	Version          string
	Repository       string
	InstallationType string
}

func NewLoadOptions(input LoadOptionsInput) config.LoadOptions {
	return config.LoadOptions{
		ConfigPath:        input.ConfigFile,
		Namespace:         input.Namespace,
		UserOverridesPath: DefaultOverridesPath,
		Profile:           input.Profile,
		Version:           input.Version,
		Repository:        input.Repository,
		InstallationType:  input.InstallationType,
	}
}

func EnsureNamespace(namespace string) string {
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}

func EnsureStateName(name string) string {
	if name == "" {
		return state.DefaultInstallationName
	}
	return name
}

func DefaultStateStoreFactory(cfg *rest.Config, namespace string) (state.Store, error) {
	return state.NewStore(cfg, namespace)
}
