package modules

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/google/uuid"
	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/plugins"
	"github.com/neutrome-labs/open-ai-router/src/sse"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// OpenAIResponsesModule handles OpenAI Responses API requests.
type OpenAIResponsesModule struct {
	RouterName string `json:"router,omitempty"`
	logger     *zap.Logger
}

// responsesInvoker implements plugins.HandlerInvoker for recursive handler calls.
type responsesInvoker struct {
	module  *OpenAIResponsesModule
	router  *RouterModule
	chain   *plugins.PluginChain
	reqBody []byte
	r       *http.Request
}

// InvokeHandler invokes the handler with a modified request, writing to the ResponseWriter.
func (inv *responsesInvoker) InvokeHandler(w http.ResponseWriter, r *http.Request, req formats.ManagedRequest) error {
	return inv.module.handleRequest(inv.router, inv.chain, inv.reqBody, w, r, req.(*formats.OpenAIResponsesRequest))
}

// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
func (inv *responsesInvoker) InvokeHandlerCapture(r *http.Request, req formats.ManagedRequest) (formats.ManagedResponse, error) {
	capture := &responsesCaptureWriter{}
	err := inv.module.handleRequest(inv.router, inv.chain, inv.reqBody, capture, r, req.(*formats.OpenAIResponsesRequest))
	if err != nil {
		return nil, err
	}
	return capture.response, nil
}

// responsesCaptureWriter captures response instead of writing to HTTP
type responsesCaptureWriter struct {
	response formats.ManagedResponse
	headers  http.Header
}

func (w *responsesCaptureWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *responsesCaptureWriter) Write(data []byte) (int, error) {
	resp := &formats.OpenAIResponsesResponse{}
	if err := resp.FromJSON(data); err != nil {
		return 0, err
	}
	w.response = resp
	return len(data), nil
}

func (w *responsesCaptureWriter) WriteHeader(statusCode int) {}

func ParseOpenAIResponsesModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m OpenAIResponsesModule
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "router":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				m.RouterName = h.Val()
			default:
				return nil, h.Errf("unrecognized ai_openai_responses option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (*OpenAIResponsesModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_openai_responses",
		New: func() caddy.Module { return new(OpenAIResponsesModule) },
	}
}

func (m *OpenAIResponsesModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)
	return nil
}

func (m *OpenAIResponsesModule) resolvePlugins(r *http.Request, req formats.ManagedRequest) *plugins.PluginChain {
	chain := plugins.NewPluginChain()

	for _, mp := range plugins.HeadPlugins {
		if p, ok := plugins.GetPlugin(mp[0]); ok {
			chain.Add(p, mp[1])
		}
	}

	// Plugins from path
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" {
		pathParts := strings.Split(path, "/")
		for _, part := range pathParts {
			if part == "" {
				continue
			}
			if idx := strings.IndexByte(part, ':'); idx > 0 {
				if p, ok := plugins.GetPlugin(part[:idx]); ok {
					chain.Add(p, part[idx+1:])
				}
			} else if part != "" {
				if p, ok := plugins.GetPlugin(part); ok {
					chain.Add(p, "")
				}
			}
		}
	}

	for _, mp := range plugins.TailPlugins {
		if p, ok := plugins.GetPlugin(mp[0]); ok {
			chain.Add(p, mp[1])
		}
	}

	return chain
}

func (m *OpenAIResponsesModule) serveResponses(
	p *ProviderDef,
	cmd drivers.InferenceCommand,
	req formats.ManagedRequest,
	originalBody []byte,
	chain *plugins.PluginChain,
	w http.ResponseWriter,
	r *http.Request,
) error {
	inputStyle := styles.StyleOpenAIResponses
	outputStyle := p.impl.Style

	// Merge original request extras for passthrough of unknown fields
	if err := req.MergeFrom(originalBody); err != nil {
		m.logger.Warn("Failed to merge original request extras", zap.Error(err))
	}

	// Convert request format (passthrough if same style)
	converter := &styles.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(req, inputStyle, outputStyle)
	if err != nil {
		m.logger.Error("Failed to convert request format", zap.Error(err))
		http.Error(w, "Format conversion error", http.StatusInternalServerError)
		return nil
	}

	hres, res, err := cmd.DoInference(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("inference error", zap.String("provider", p.Name), zap.Error(err))
		// Run error plugins to notify about the failure
		_ = chain.RunError(&p.impl, r, req, hres, err)
		return err
	}

	// Convert response back to input style (passthrough if same style)
	if res != nil {
		res, err = converter.ConvertResponse(res, outputStyle, inputStyle)
		if err != nil {
			m.logger.Error("Failed to convert response format", zap.Error(err))
		}
	}

	res, err = chain.RunAfter(&p.impl, r, req, hres, res)
	if err != nil {
		m.logger.Error("plugin after hook error", zap.Error(err))
		http.Error(w, "Plugin error", http.StatusInternalServerError)
		return nil
	}

	data, err := res.ToJSON()
	if err != nil {
		m.logger.Error("Failed to serialize response", zap.Error(err))
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}

func (m *OpenAIResponsesModule) serveResponsesStream(
	p *ProviderDef,
	cmd drivers.InferenceCommand,
	req formats.ManagedRequest,
	originalBody []byte,
	chain *plugins.PluginChain,
	w http.ResponseWriter,
	r *http.Request,
) error {
	sseWriter := sse.NewWriter(w)

	if err := sseWriter.WriteHeartbeat("ok"); err != nil {
		return err
	}

	inputStyle := styles.StyleOpenAIResponses
	outputStyle := p.impl.Style

	// Merge original request extras for passthrough of unknown fields
	if err := req.MergeFrom(originalBody); err != nil {
		m.logger.Warn("Failed to merge original request extras", zap.Error(err))
	}

	// Convert request format (passthrough if same style)
	converter := &styles.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(req, inputStyle, outputStyle)
	if err != nil {
		m.logger.Error("Failed to convert request format", zap.Error(err))
		_ = sseWriter.WriteError("Format conversion error")
		_ = sseWriter.WriteDone()
		return nil
	}

	hres, stream, err := cmd.DoInferenceStream(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("inference stream error", zap.String("provider", p.Name), zap.Error(err))
		// Run error plugins to notify about the failure
		_ = chain.RunError(&p.impl, r, req, hres, err)
		_ = sseWriter.WriteError("start failed")
		_ = sseWriter.WriteDone()
		return err
	}

	var lastChunk formats.ManagedResponse

	for chunk := range stream {
		if chunk.RuntimeError != nil {
			_ = sseWriter.WriteError(chunk.RuntimeError.Error())
			// Run error plugins for runtime stream errors
			_ = chain.RunError(&p.impl, r, req, hres, chunk.RuntimeError)
			return nil
		}

		chunkData := chunk.Data

		// Convert chunk (passthrough if same style)
		if chunkData != nil {
			converted, err := converter.ConvertResponse(chunkData, outputStyle, inputStyle)
			if err == nil {
				chunkData = converted
			}
		}

		chunkData, err = chain.RunAfterChunk(&p.impl, r, req, hres, chunkData)
		if err != nil {
			m.logger.Error("plugin after chunk error", zap.Error(err))
			continue
		}

		if chunkData != nil {
			lastChunk = chunkData
			data, err := chunkData.ToJSON()
			if err != nil {
				continue
			}
			if err := sseWriter.WriteRaw(data); err != nil {
				return err
			}
		}
	}

	_ = chain.RunStreamEnd(&p.impl, r, req, hres, lastChunk)
	_ = sseWriter.WriteDone()
	return nil
}

func (m *OpenAIResponsesModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil
	}

	req := &formats.OpenAIResponsesRequest{}
	if err := req.FromJSON(reqBody); err != nil {
		m.logger.Error("failed to parse request body", zap.Error(err))
		http.Error(w, "failed to parse request body", http.StatusBadRequest)
		return nil
	}

	router, ok := GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("Router not found", zap.String("name", m.RouterName))
		http.Error(w, "Router not found", http.StatusInternalServerError)
		return nil
	}

	// Collect incoming auth early so plugins can rely on context values
	r, err = router.impl.AuthManager.CollectIncomingAuth(r)
	if err != nil {
		m.logger.Error("failed to collect incoming auth", zap.Error(err))
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return nil
	}

	chain := m.resolvePlugins(r, req)

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), plugins.ContextTraceID(), traceId))

	// Create invoker for recursive handler plugins
	invoker := &responsesInvoker{
		module:  m,
		router:  router,
		chain:   chain,
		reqBody: reqBody,
		r:       r,
	}

	// Check if any recursive handler plugin wants to handle this request
	handled, err := chain.RunRecursiveHandlers(invoker, w, r, req)
	if handled {
		if err != nil {
			m.logger.Error("recursive handler plugin failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	// Normal flow - handle request directly
	err = m.handleRequest(router, chain, reqBody, w, r, req)
	if err != nil {
		m.logger.Error("request handling failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	return nil
}

// handleRequest handles a single request to providers (used both directly and by recursive plugins).
func (m *OpenAIResponsesModule) handleRequest(
	router *RouterModule,
	chain *plugins.PluginChain,
	reqBody []byte,
	w http.ResponseWriter,
	r *http.Request,
	req *formats.OpenAIResponsesRequest,
) error {
	providers, model := router.ResolveProvidersOrderAndModel(req.GetModel())
	req.SetModel(model)

	var displayErr error
	for _, name := range providers {
		p, ok := router.Providers[name]
		if !ok {
			continue
		}

		// Clone request for isolated plugin processing per provider
		providerReq := req.Clone().(*formats.OpenAIResponsesRequest)

		// Run before plugins with provider context
		processedReq, err := chain.RunBefore(&p.impl, r, providerReq)
		if err != nil {
			m.logger.Error("plugin before hook error", zap.String("provider", name), zap.Error(err))
			if displayErr == nil {
				displayErr = err
			}
			continue
		}
		providerReq = processedReq.(*formats.OpenAIResponsesRequest)

		cmd, ok := p.impl.Commands["inference"].(drivers.InferenceCommand)
		if !ok {
			continue
		}

		if providerReq.IsStreaming() {
			err = m.serveResponsesStream(p, cmd, providerReq, reqBody, chain, w, r)
		} else {
			err = m.serveResponses(p, cmd, providerReq, reqBody, chain, w, r)
		}

		if err != nil {
			if displayErr == nil {
				displayErr = err
			}
			continue
		}

		w.Header().Set("X-Real-Provider-Id", name)
		w.Header().Set("X-Real-Model-Id", model)
		return nil
	}

	if displayErr != nil {
		return displayErr
	}

	return nil
}

var (
	_ caddy.Provisioner           = (*OpenAIResponsesModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*OpenAIResponsesModule)(nil)
)
