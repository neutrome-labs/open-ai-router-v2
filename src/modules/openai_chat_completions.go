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
	"github.com/neutrome-labs/open-ai-router/src/drivers/openai"
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/plugins"
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
	// Share logger with other components for consistent debug output
	plugins.Logger = m.logger
	styles.Logger = m.logger
	openai.Logger = m.logger
	return nil
}

func (m *OpenAIChatCompletionsModule) resolvePlugins(r *http.Request, req formats.ManagedRequest) *plugins.PluginChain {
	chain := plugins.NewPluginChain()

	// Add mandatory plugins
	for _, mp := range plugins.MandatoryPlugins {
		if p, ok := plugins.GetPlugin(mp[0]); ok {
			chain.Add(p, mp[1])
		}
	}

	// Plugins from path: /v1/chat/completions/plugin1:arg1/plugin2:arg2
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

	// Plugins from model suffix: model="gpt-4+plugin1:arg1+plugin2"
	modelStr := req.GetModel()
	if idx := strings.IndexByte(modelStr, '+'); idx >= 0 {
		pluginPart := modelStr[idx+1:]
		for len(pluginPart) > 0 {
			var part string
			if nextIdx := strings.IndexByte(pluginPart, '+'); nextIdx >= 0 {
				part = pluginPart[:nextIdx]
				pluginPart = pluginPart[nextIdx+1:]
			} else {
				part = pluginPart
				pluginPart = ""
			}
			if part == "" {
				continue
			}
			if colonIdx := strings.IndexByte(part, ':'); colonIdx >= 0 {
				name := part[:colonIdx]
				if name != "" {
					if p, ok := plugins.GetPlugin(name); ok {
						chain.Add(p, part[colonIdx+1:])
					}
				}
			} else {
				if p, ok := plugins.GetPlugin(part); ok {
					chain.Add(p, "")
				}
			}
		}
	}

	return chain
}

func (m *OpenAIChatCompletionsModule) serveChatCompletions(
	p *ProviderDef,
	cmd drivers.ChatCompletionsCommand,
	req formats.ManagedRequest,
	originalBody []byte,
	chain *plugins.PluginChain,
	w http.ResponseWriter,
	r *http.Request,
) error {
	inputStyle := styles.StyleOpenAIChat
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

	hres, res, err := cmd.DoChatCompletions(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("chat completions error", zap.String("provider", p.Name), zap.Error(err))
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

	// Run after plugins
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

func (m *OpenAIChatCompletionsModule) serveChatCompletionsStream(
	p *ProviderDef,
	cmd drivers.ChatCompletionsCommand,
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

	inputStyle := styles.StyleOpenAIChat
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

	hres, stream, err := cmd.DoChatCompletionsStream(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("chat completions stream error (start)", zap.String("provider", p.Name), zap.Error(err))
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

		// Run after-chunk plugins
		chunkData, err = chain.RunAfterChunk(&p.impl, r, req, hres, chunkData)
		if err != nil {
			m.logger.Error("plugin after chunk error", zap.Error(err))
			continue
		}

		if chunkData != nil {
			lastChunk = chunkData
			data, err := chunkData.ToJSON()
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
	_ = chain.RunStreamEnd(&p.impl, r, req, hres, lastChunk)

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

	req := &formats.OpenAIChatRequest{}
	if err := req.FromJSON(reqBody); err != nil {
		m.logger.Error("failed to parse request body", zap.Error(err))
		http.Error(w, "failed to parse request body", http.StatusBadRequest)
		return nil
	}

	m.logger.Debug("Request parsed",
		zap.String("model", req.GetModel()),
		zap.Bool("streaming", req.IsStreaming()),
		zap.Int("messages", len(req.Messages)))

	router, ok := GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("Router not found", zap.String("name", m.RouterName))
		http.Error(w, "Router not found", http.StatusInternalServerError)
		return nil
	}

	chain := m.resolvePlugins(r, req)
	providers, model := router.ResolveProvidersOrderAndModel(req.GetModel())
	req.SetModel(model)

	m.logger.Debug("Resolved providers",
		zap.String("model", model),
		zap.Strings("providers", providers),
		zap.Int("plugin_count", len(chain.GetPlugins())))

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), plugins.ContextTraceID(), traceId))

	var displayErr error
	for _, name := range providers {
		m.logger.Debug("Trying provider", zap.String("provider", name))

		p, ok := router.Providers[name]
		if !ok {
			m.logger.Error("provider not found", zap.String("name", name))
			continue
		}

		cmd, ok := p.impl.Commands["chat_completions"].(drivers.ChatCompletionsCommand)
		if !ok {
			m.logger.Debug("Provider does not support chat_completions", zap.String("provider", name))
			continue
		}

		// Clone request for isolated plugin processing per provider
		providerReq := req.Clone().(*formats.OpenAIChatRequest)

		// Run before plugins with provider context
		processedReq, err := chain.RunBefore(&p.impl, r, providerReq)
		if err != nil {
			m.logger.Error("plugin before hook error", zap.String("provider", name), zap.Error(err))
			if displayErr == nil {
				displayErr = err
			}
			continue
		}
		providerReq = processedReq.(*formats.OpenAIChatRequest)

		m.logger.Debug("Executing chat completions",
			zap.String("provider", name),
			zap.String("style", string(p.impl.Style)),
			zap.Bool("streaming", providerReq.IsStreaming()))

		if providerReq.IsStreaming() {
			err = m.serveChatCompletionsStream(p, cmd, providerReq, reqBody, chain, w, r)
		} else {
			err = m.serveChatCompletions(p, cmd, providerReq, reqBody, chain, w, r)
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
		m.logger.Error("all providers failed", zap.String("model", model), zap.Error(displayErr))
		http.Error(w, displayErr.Error(), http.StatusInternalServerError)
		return nil
	}

	return next.ServeHTTP(w, r)
}

var (
	_ caddy.Provisioner           = (*OpenAIChatCompletionsModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*OpenAIChatCompletionsModule)(nil)
)
