package config

import "embed"

//go:embed crd/bases/*.yaml
var CRDFiles embed.FS
