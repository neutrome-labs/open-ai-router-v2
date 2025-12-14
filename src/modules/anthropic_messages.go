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

// AnthropicMessagesModule handles Anthropic Messages API requests.
type AnthropicMessagesModule struct {
	RouterName string `json:"router,omitempty"`
	logger     *zap.Logger
}

func ParseAnthropicMessagesModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m AnthropicMessagesModule
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "router":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				m.RouterName = h.Val()
			default:
				return nil, h.Errf("unrecognized ai_anthropic_messages option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (*AnthropicMessagesModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_anthropic_messages",
		New: func() caddy.Module { return new(AnthropicMessagesModule) },
	}
}

func (m *AnthropicMessagesModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)
	return nil
}

func (m *AnthropicMessagesModule) resolvePlugins(r *http.Request, req formats.ManagedRequest) *plugins.PluginChain {
	chain := plugins.NewPluginChain()

	for _, mp := range plugins.MandatoryPlugins {
		if p, ok := plugins.GetPlugin(mp[0]); ok {
			chain.Add(p, mp[1])
		}
	}

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

	return chain
}

func (m *AnthropicMessagesModule) serveMessages(
	p *ProviderDef,
	cmd drivers.InferenceCommand,
	req formats.ManagedRequest,
	originalBody []byte,
	chain *plugins.PluginChain,
	w http.ResponseWriter,
	r *http.Request,
) error {
	inputStyle := styles.StyleAnthropic
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

func (m *AnthropicMessagesModule) serveMessagesStream(
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

	inputStyle := styles.StyleAnthropic
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

func (m *AnthropicMessagesModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil
	}

	req := &formats.AnthropicRequest{}
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

	chain := m.resolvePlugins(r, req)
	providers, model := router.ResolveProvidersOrderAndModel(req.GetModel())
	req.SetModel(model)

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), plugins.ContextTraceID(), traceId))

	var displayErr error
	for _, name := range providers {
		p, ok := router.Providers[name]
		if !ok {
			continue
		}

		// Clone request for isolated plugin processing per provider
		providerReq := req.Clone().(*formats.AnthropicRequest)

		// Run before plugins with provider context
		processedReq, err := chain.RunBefore(&p.impl, r, providerReq)
		if err != nil {
			m.logger.Error("plugin before hook error", zap.String("provider", name), zap.Error(err))
			if displayErr == nil {
				displayErr = err
			}
			continue
		}
		providerReq = processedReq.(*formats.AnthropicRequest)

		// Use unified inference command
		cmd, ok := p.impl.Commands["inference"].(drivers.InferenceCommand)
		if !ok {
			continue
		}

		if providerReq.IsStreaming() {
			err = m.serveMessagesStream(p, cmd, providerReq, reqBody, chain, w, r)
		} else {
			err = m.serveMessages(p, cmd, providerReq, reqBody, chain, w, r)
		}

		w.Header().Set("X-Real-Provider-Id", name)
		w.Header().Set("X-Real-Model-Id", model)
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
	_ caddy.Provisioner           = (*AnthropicMessagesModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*AnthropicMessagesModule)(nil)
)
