package rlarkframework

import (
	"fmt"
	"os"

	shared "github.com/rlinf/rlark/test/e2e/framework"
	kindprovider "github.com/rlinf/rlark/test/e2e/framework/kind"
)

const (
	LabelPrefix  = "e2e.rlark.io"
	ProviderKind = kindprovider.ProviderName
)

type Config struct {
	Shared   shared.Config
	Provider string
	Kind     kindprovider.Config
}

func LoadConfig() (Config, error) {
	common, err := shared.LoadConfig()
	if err != nil {
		return Config{}, err
	}
	config := Config{
		Shared:   common,
		Provider: ProviderKind,
		Kind: kindprovider.Config{
			Binary:    shared.EnvOrDefault("RLARK_E2E_KIND_BINARY", "kind"),
			NodeImage: os.Getenv("RLARK_E2E_KIND_NODE_IMAGE"),
		},
	}
	return config, nil
}

func (c Config) NewProvider(requirements shared.Requirements) (shared.Provider, error) {
	if len(requirements.Clusters) == 0 {
		return nil, fmt.Errorf("RLark scenario must declare at least one cluster")
	}
	if c.Provider != ProviderKind {
		return nil, fmt.Errorf("unsupported RLark E2E provider %q", c.Provider)
	}
	return kindprovider.New(c.Kind), nil
}
