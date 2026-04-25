package marmotd

import (
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// assignNodeNameIfUnset sets Metadata.nodeName only when it is unset.
// This keeps room for future schedulers to pre-assign nodeName in requests.
func assignNodeNameIfUnset(metadata **api.Metadata, nodeName string) {
	if strings.TrimSpace(nodeName) == "" {
		return
	}
	if *metadata == nil {
		*metadata = &api.Metadata{}
	}
	if (*metadata).NodeName == nil || strings.TrimSpace(*(*metadata).NodeName) == "" {
		(*metadata).NodeName = util.StringPtr(nodeName)
	}
}
