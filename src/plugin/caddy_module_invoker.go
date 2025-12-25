package plugin

import (
	"net/http"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// CaddyModuleInvoker invokes Caddy HTTP modules as plugins
type CaddyModuleInvoker struct {
	module caddyhttp.MiddlewareHandler
}

func NewCaddyModuleInvoker(module caddyhttp.MiddlewareHandler) *CaddyModuleInvoker {
	return &CaddyModuleInvoker{
		module: module,
	}
}

// InvokeHandler invokes the handler with a modified request, writing to the ResponseWriter.
func (inv *CaddyModuleInvoker) InvokeHandler(w http.ResponseWriter, r *http.Request) error {
	return inv.module.ServeHTTP(w, r, nil)
}

// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
func (inv *CaddyModuleInvoker) InvokeHandlerCapture(r *http.Request) (styles.PartialJSON, error) {
	// Create a response capture writer
	capture := &services.ResponseCaptureWriter{}
	err := inv.module.ServeHTTP(capture, r, nil)
	if err != nil {
		return nil, err
	}
	if capture.Response == nil {
		return nil, nil
	}
	return styles.ParsePartialJSON(capture.Response)
}
