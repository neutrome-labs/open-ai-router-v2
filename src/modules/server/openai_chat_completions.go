package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/google/uuid"
	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/modules"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/sse"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// OpenAIChatCompletionsModule handles OpenAI-style chat completions requests.
// V3 upgrade: Supports passthrough when input/output styles match, minimizes serialization.
type OpenAIChatCompletionsModule struct {
	RouterName string `json:"router,omitempty"`
	logger     *zap.Logger
}

type KnownOpenAIChatRequest struct {
	Stream bool   `json:"stream"`
	Model  string `json:"model"`
}

// responseCaptureWriter captures response instead of writing to HTTP
type responseCaptureWriter struct {
	response json.RawMessage
	headers  http.Header
}

func (w *responseCaptureWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	w.response = json.RawMessage(data)
	return len(data), nil
}

func (w *responseCaptureWriter) WriteHeader(statusCode int) {
	// Ignore for capture
}

// chatCompletionsInvoker implements plugins.HandlerInvoker for recursive handler calls.
type chatCompletionsInvoker struct {
	module *OpenAIChatCompletionsModule
	router *modules.RouterModule
}

// InvokeHandler invokes the handler with a modified request, writing to the ResponseWriter.
func (inv *chatCompletionsInvoker) InvokeHandler(w http.ResponseWriter, r *http.Request) error {
	return inv.module.ServeHTTP(w, r, nil)
}

// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
func (inv *chatCompletionsInvoker) InvokeHandlerCapture(r *http.Request) ([]byte, error) {
	// Create a response capture writer
	capture := &responseCaptureWriter{}
	err := inv.module.ServeHTTP(capture, r, nil)
	if err != nil {
		return nil, err
	}
	if capture.response == nil {
		return nil, nil
	}
	return capture.response, nil
}

func ParseOpenAIChatCompletionsModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m OpenAIChatCompletionsModule
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "router":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				m.RouterName = h.Val()
			default:
				return nil, h.Errf("unrecognized ai_openai_chat_completions option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (*OpenAIChatCompletionsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_openai_chat_completions",
		New: func() caddy.Module { return new(OpenAIChatCompletionsModule) },
	}
}

func (m *OpenAIChatCompletionsModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)
	return nil
}

func (m *OpenAIChatCompletionsModule) serveChatCompletions(
	p *modules.ProviderConfig,
	cmd drivers.InferenceCommand,
	chain *plugin.PluginChain,
	reqBody []byte,
	w http.ResponseWriter,
	r *http.Request,
) error {
	inputStyle := styles.StyleOpenAIChat
	outputStyle := p.Impl.Style

	// Convert request format (passthrough if same style)
	converter := &styles.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(reqBody, inputStyle, outputStyle)
	if err != nil {
		m.logger.Error("Failed to convert request format", zap.Error(err))
		http.Error(w, "Format conversion error", http.StatusInternalServerError)
		return nil
	}

	res, resBody, err := cmd.DoInference(&p.Impl, providerReq, r)
	if err != nil {
		m.logger.Error("inference error", zap.String("provider", p.Name), zap.Error(err))
		// Run error plugins to notify about the failure
		_ = chain.RunError(&p.Impl, r, reqBody, res, err)
		return err
	}

	// Convert response back to input style (passthrough if same style)
	if resBody != nil {
		resBody, err = converter.ConvertResponse(resBody, outputStyle, inputStyle)
		if err != nil {
			m.logger.Error("Failed to convert response format", zap.Error(err))
		}
	}

	// Run after plugins
	resBody, err = chain.RunAfter(&p.Impl, r, reqBody, res, resBody)
	if err != nil {
		m.logger.Error("plugin after hook error", zap.Error(err))
		http.Error(w, "Plugin error", http.StatusInternalServerError)
		return nil
	}

	data, err := json.Marshal(resBody)
	if err != nil {
		m.logger.Error("Failed to serialize response", zap.Error(err))
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}

func (m *OpenAIChatCompletionsModule) serveChatCompletionsStream(
	p *modules.ProviderConfig,
	cmd drivers.InferenceCommand,
	chain *plugin.PluginChain,
	reqBody []byte,
	w http.ResponseWriter,
	r *http.Request,
) error {
	sseWriter := sse.NewWriter(w)

	if err := sseWriter.WriteHeartbeat("ok"); err != nil {
		return err
	}

	inputStyle := styles.StyleOpenAIChat
	outputStyle := p.Impl.Style

	// Convert request format (passthrough if same style)
	converter := &styles.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(reqBody, inputStyle, outputStyle)
	if err != nil {
		m.logger.Error("Failed to convert request format", zap.Error(err))
		_ = sseWriter.WriteError("Format conversion error")
		_ = sseWriter.WriteDone()
		return nil
	}

	hres, stream, err := cmd.DoInferenceStream(&p.Impl, providerReq, r)
	if err != nil {
		m.logger.Error("inference stream error (start)", zap.String("provider", p.Name), zap.Error(err))
		// Run error plugins to notify about the failure
		_ = chain.RunError(&p.Impl, r, reqBody, hres, err)
		_ = sseWriter.WriteError("start failed")
		_ = sseWriter.WriteDone()
		return err
	}

	var lastChunk json.RawMessage

	for chunk := range stream {
		if chunk.RuntimeError != nil {
			_ = sseWriter.WriteError(chunk.RuntimeError.Error())
			// Run error plugins for runtime stream errors
			_ = chain.RunError(&p.Impl, r, reqBody, hres, chunk.RuntimeError)
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

		// Run after-chunk plugins
		chunkData, err = chain.RunAfterChunk(&p.Impl, r, reqBody, hres, chunkData)
		if err != nil {
			m.logger.Error("plugin after chunk error", zap.Error(err))
			continue
		}

		if chunkData != nil {
			lastChunk = chunkData
			data, err := json.Marshal(chunkData)
			if err != nil {
				m.logger.Error("Failed to serialize chunk", zap.Error(err))
				continue
			}
			if err := sseWriter.WriteRaw(data); err != nil {
				m.logger.Error("chat completions stream write error", zap.Error(err))
				return err
			}
		}
	}

	// Run stream end plugins
	_ = chain.RunStreamEnd(&p.Impl, r, reqBody, hres, lastChunk)

	_ = sseWriter.WriteDone()
	return nil
}

func (m *OpenAIChatCompletionsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	m.logger.Debug("Chat completions request received", zap.String("path", r.URL.Path), zap.String("method", r.Method))

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil
	}

	m.logger.Debug("Request body read", zap.Int("body_length", len(reqBody)))

	knownReq := &KnownOpenAIChatRequest{}
	if err := json.Unmarshal(reqBody, knownReq); err != nil {
		m.logger.Error("failed to parse known request fields", zap.Error(err))
		http.Error(w, "failed to parse request body", http.StatusBadRequest)
		return nil
	}

	m.logger.Debug("Request parsed",
		zap.String("model", knownReq.Model),
		zap.Bool("streaming", knownReq.Stream))

	router, ok := modules.GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("Router not found", zap.String("name", m.RouterName))
		http.Error(w, "Router not found", http.StatusInternalServerError)
		return nil
	}

	// Collect incoming auth early so plugins can rely on context values
	r, err = router.Impl.Auth.CollectIncomingAuth(r)
	if err != nil {
		m.logger.Error("failed to collect incoming auth", zap.Error(err))
		http.Error(w, "authentication error", http.StatusUnauthorized)
		return nil
	}

	chain := plugin.TryResolvePlugins(*r.URL, knownReq.Model)

	m.logger.Debug("Resolved plugins", zap.Int("plugin_count", len(chain.GetPlugins())))

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), plugin.ContextTraceID(), traceId))

	// Create invoker for recursive handler plugins
	invoker := &chatCompletionsInvoker{
		module: m,
		router: router,
	}

	// Check if any recursive handler plugin wants to handle this request
	handled, err := chain.RunRecursiveHandlers(invoker, reqBody, w, r)
	if handled {
		if err != nil {
			m.logger.Error("recursive handler plugin failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	// Normal flow - handle request directly
	err = m.handleRequest(router, chain, reqBody, *knownReq, w, r)
	if err != nil {
		m.logger.Error("request handling failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	return nil
}

// handleRequest handles a single request to providers (used both directly and by recursive plugins).
func (m *OpenAIChatCompletionsModule) handleRequest(
	router *modules.RouterModule,
	chain *plugin.PluginChain,
	reqBody []byte,
	knownReq KnownOpenAIChatRequest,
	w http.ResponseWriter,
	r *http.Request,
) error {
	providers, model := router.ResolveProvidersOrderAndModel(knownReq.Model)
	knownReq.Model = model

	m.logger.Debug("Resolved providers",
		zap.String("model", model),
		zap.Strings("providers", providers),
		zap.Int("plugin_count", len(chain.GetPlugins())))

	var displayErr error
	for _, name := range providers {
		m.logger.Debug("Trying provider", zap.String("provider", name))

		p, ok := router.ProviderConfigs[name]
		if !ok {
			m.logger.Error("provider not found", zap.String("name", name))
			continue
		}

		cmd, ok := p.Impl.Commands["inference"].(drivers.InferenceCommand)
		if !ok {
			m.logger.Debug("Provider does not support inference", zap.String("provider", name))
			continue
		}

		// Clone request for isolated plugin processing per provider
		providerReq := make([]byte, len(reqBody))
		copy(providerReq, reqBody)

		// Run before plugins with provider context
		processedReq, err := chain.RunBefore(&p.Impl, r, providerReq)
		if err != nil {
			m.logger.Error("plugin before hook error", zap.String("provider", name), zap.Error(err))
			if displayErr == nil {
				displayErr = err
			}
			continue
		}
		providerReq = processedReq

		m.logger.Debug("Executing inference",
			zap.String("provider", name),
			zap.String("style", string(p.Impl.Style)),
			zap.Bool("streaming", knownReq.Stream))

		if knownReq.Stream {
			err = m.serveChatCompletionsStream(p, cmd, chain, providerReq, w, r)
		} else {
			err = m.serveChatCompletions(p, cmd, chain, providerReq, w, r)
		}

		if err != nil {
			if displayErr == nil {
				displayErr = err
			}
			continue
		}

		// Success - set response headers
		w.Header().Set("X-Real-Provider-Id", name)
		w.Header().Set("X-Real-Model-Id", model)

		// Build plugin list for header
		var pluginNames []string
		for _, pi := range chain.GetPlugins() {
			name := pi.Plugin.Name()
			if pi.Params != "" {
				name += ":" + pi.Params
			}
			pluginNames = append(pluginNames, name)
		}
		w.Header().Set("X-Plugins-Executed", strings.Join(pluginNames, ","))

		return nil
	}

	if displayErr != nil {
		return displayErr
	}

	return nil
}

var (
	_ caddy.Provisioner           = (*OpenAIChatCompletionsModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*OpenAIChatCompletionsModule)(nil)
)
