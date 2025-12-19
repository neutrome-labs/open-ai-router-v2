package server

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
	"github.com/neutrome-labs/open-ai-router/src/drivers/openai"
	"github.com/neutrome-labs/open-ai-router/src/drivers/virtual"
	"github.com/neutrome-labs/open-ai-router/src/modules"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/plugins"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/sse"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// ChatCompletionsModule handles OpenAI-style chat completions requests.
// V3 upgrade: Supports passthrough when input/output styles match, minimizes serialization.
type ChatCompletionsModule struct {
	RouterName string `json:"router,omitempty"`
	logger     *zap.Logger
}

// responseCaptureWriter captures response instead of writing to HTTP
type responseCaptureWriter struct {
	response []byte
	headers  http.Header
}

func (w *responseCaptureWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	w.response = data
	return len(data), nil
}

func (w *responseCaptureWriter) WriteHeader(statusCode int) {
	// Ignore for capture
}

// chatCompletionsInvoker implements plugins.HandlerInvoker for recursive handler calls.
type chatCompletionsInvoker struct {
	module *ChatCompletionsModule
	router *modules.RouterModule
}

// InvokeHandler invokes the handler with a modified request, writing to the ResponseWriter.
func (inv *chatCompletionsInvoker) InvokeHandler(w http.ResponseWriter, r *http.Request) error {
	return inv.module.ServeHTTP(w, r, nil)
}

// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
func (inv *chatCompletionsInvoker) InvokeHandlerCapture(r *http.Request) (styles.PartialJSON, error) {
	// Create a response capture writer
	capture := &responseCaptureWriter{}
	err := inv.module.ServeHTTP(capture, r, nil)
	if err != nil {
		return nil, err
	}
	if capture.response == nil {
		return nil, nil
	}
	return styles.ParsePartialJSON(capture.response)
}

func ParseChatCompletionsModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m ChatCompletionsModule
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

func (*ChatCompletionsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_openai_chat_completions",
		New: func() caddy.Module { return new(ChatCompletionsModule) },
	}
}

func (m *ChatCompletionsModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)

	// Provision package-level loggers for all subsystems
	plugin.Logger = m.logger.Named("plugin")
	plugins.Logger = m.logger.Named("plugins")
	styles.Logger = m.logger.Named("styles")
	openai.Logger = m.logger.Named("openai")
	virtual.Logger = m.logger.Named("virtual")

	return nil
}

func (m *ChatCompletionsModule) serveChatCompletions(
	p *modules.ProviderConfig,
	cmd drivers.InferenceCommand,
	chain *plugin.PluginChain,
	reqJson styles.PartialJSON,
	w http.ResponseWriter,
	r *http.Request,
) error {
	inputStyle := styles.StyleChatCompletions
	outputStyle := p.Impl.Style

	// Convert request format (passthrough if same style)
	converter := &services.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(reqJson, inputStyle, outputStyle)
	if err != nil {
		m.logger.Error("Failed to convert request format", zap.Error(err))
		http.Error(w, "Format conversion error", http.StatusInternalServerError)
		return nil
	}

	res, resJson, err := cmd.DoInference(&p.Impl, providerReq, r)
	if err != nil {
		m.logger.Error("inference error", zap.String("provider", p.Name), zap.Error(err))
		// Run error plugins to notify about the failure
		_ = chain.RunError(&p.Impl, r, reqJson, res, err)
		return err
	}

	// Convert response back to input style (passthrough if same style)
	if resJson != nil {
		resJson, err = converter.ConvertResponse(resJson, outputStyle, inputStyle)
		if err != nil {
			m.logger.Error("Failed to convert response format", zap.Error(err))
		}
	}

	// Run after plugins
	resJson, err = chain.RunAfter(&p.Impl, r, reqJson, res, resJson)
	if err != nil {
		m.logger.Error("plugin after hook error", zap.Error(err))
		http.Error(w, "Plugin error", http.StatusInternalServerError)
		return nil
	}

	resData, err := resJson.Marshal()
	if err != nil {
		m.logger.Error("Failed to serialize response JSON", zap.Error(err))
		http.Error(w, "Response serialization error", http.StatusInternalServerError)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(resData)
	return err
}

func (m *ChatCompletionsModule) serveChatCompletionsStream(
	p *modules.ProviderConfig,
	cmd drivers.InferenceCommand,
	chain *plugin.PluginChain,
	reqJson styles.PartialJSON,
	w http.ResponseWriter,
	r *http.Request,
) error {
	sseWriter := sse.NewWriter(w)

	if err := sseWriter.WriteHeartbeat("ok"); err != nil {
		return err
	}

	inputStyle := styles.StyleChatCompletions
	outputStyle := p.Impl.Style

	// Convert request format (passthrough if same style)
	converter := &services.DefaultConverter{}
	providerReq, err := converter.ConvertRequest(reqJson, inputStyle, outputStyle)
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
		_ = chain.RunError(&p.Impl, r, reqJson, hres, err)
		_ = sseWriter.WriteError("start failed")
		_ = sseWriter.WriteDone()
		return err
	}

	var lastChunk styles.PartialJSON

	for chunk := range stream {
		if chunk.RuntimeError != nil {
			_ = sseWriter.WriteError(chunk.RuntimeError.Error())
			// Run error plugins for runtime stream errors
			_ = chain.RunError(&p.Impl, r, reqJson, hres, chunk.RuntimeError)
			return nil
		}

		chunkJson := chunk.Data

		// Convert chunk (passthrough if same style)
		if chunkJson != nil {
			converted, err := converter.ConvertResponseChunk(chunkJson, outputStyle, inputStyle)
			if err == nil {
				chunkJson = converted
			}
		}

		// Run after-chunk plugins
		chunkJson, err = chain.RunAfterChunk(&p.Impl, r, reqJson, hres, chunkJson)
		if err != nil {
			m.logger.Error("plugin after chunk error", zap.Error(err))
			continue
		}

		if chunkJson != nil {
			lastChunk = chunkJson

			chankData, err := chunkJson.Marshal()
			if err != nil {
				m.logger.Error("chat completions stream chunk marshal error", zap.Error(err))
				continue
			}

			if err := sseWriter.WriteRaw(chankData); err != nil {
				m.logger.Error("chat completions stream write error", zap.Error(err))
				return err
			}
		}
	}

	// Run stream end plugins
	_ = chain.RunStreamEnd(&p.Impl, r, reqJson, hres, lastChunk)

	_ = sseWriter.WriteDone()
	return nil
}

func (m *ChatCompletionsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	m.logger.Debug("Chat completions request received", zap.String("path", r.URL.Path), zap.String("method", r.Method))

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil
	}

	m.logger.Debug("Request body read", zap.Int("body_length", len(reqBody)))

	reqJson, err := styles.ParsePartialJSON(reqBody)
	if err != nil {
		m.logger.Error("failed to parse request JSON", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return nil
	}

	m.logger.Debug("Request parsed",
		zap.String("model", styles.TryGetFromPartialJSON[string](reqJson, "model")),
		zap.Bool("streaming", styles.TryGetFromPartialJSON[bool](reqJson, "stream")))

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

	chain := plugin.TryResolvePlugins(*r.URL, styles.TryGetFromPartialJSON[string](reqJson, "model"))

	m.logger.Debug("Resolved plugins", zap.Int("plugin_count", len(chain.GetPlugins())))

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), plugin.ContextTraceID(), traceId))

	// Create invoker for recursive handler plugins
	invoker := &chatCompletionsInvoker{
		module: m,
		router: router,
	}

	// Check if any recursive handler plugin wants to handle this request
	handled, err := chain.RunRecursiveHandlers(invoker, reqJson, w, r)
	if handled {
		if err != nil {
			m.logger.Error("recursive handler plugin failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	// Normal flow - handle request directly
	err = m.handleRequest(router, chain, reqJson, w, r)
	if err != nil {
		m.logger.Error("request handling failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	return nil
}

// handleRequest handles a single request to providers (used both directly and by recursive plugins).
func (m *ChatCompletionsModule) handleRequest(
	router *modules.RouterModule,
	chain *plugin.PluginChain,
	reqJson styles.PartialJSON,
	w http.ResponseWriter,
	r *http.Request,
) error {
	providers, model := router.ResolveProvidersOrderAndModel(styles.TryGetFromPartialJSON[string](reqJson, "model"))

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

		providerReq, err := reqJson.CloneWith("model", model)
		if err != nil {
			m.logger.Error("failed to clone request JSON with new model", zap.Error(err))
			continue
		}

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
			zap.Bool("streaming", styles.TryGetFromPartialJSON[bool](providerReq, "stream")))

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

		if styles.TryGetFromPartialJSON[bool](providerReq, "stream") {
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

		return nil
	}

	if displayErr != nil {
		return displayErr
	}

	return nil
}

var (
	_ caddy.Provisioner           = (*ChatCompletionsModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*ChatCompletionsModule)(nil)
)
