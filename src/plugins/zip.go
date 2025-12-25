package plugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// Zip provides oss implementation for context auto-compaction
type Zip struct {
}

func (f *Zip) Name() string { return "zip" }

func (f *Zip) Before(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error) {
	return reqJson, nil
}

var (
	_ plugin.BeforePlugin = (*Zip)(nil)
)
