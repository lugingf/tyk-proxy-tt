package version

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed VERSION
var localVersion string

var pipelineVersion string

func GetVersion() string {
	if pipelineVersion == "" {
		return fmt.Sprintf("%s-dev+0\n", strings.TrimSpace(localVersion))
	}

	return pipelineVersion
}
