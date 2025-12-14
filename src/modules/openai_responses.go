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

	for _, mp := range plugins.MandatoryPlugins {
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

	return chain
}

func (m *OpenAIResponsesModule) serveResponses(
	p *ProviderDef,
	cmd drivers.ResponsesCommand,
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

	hres, res, err := cmd.DoResponses(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("responses error", zap.String("provider", p.Name), zap.Error(err))
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
	cmd drivers.ResponsesCommand,
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

	hres, stream, err := cmd.DoResponsesStream(&p.impl, providerReq, r)
	if err != nil {
		m.logger.Error("responses stream error", zap.String("provider", p.Name), zap.Error(err))
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

		cmd, ok := p.impl.Commands["responses"].(drivers.ResponsesCommand)
		if !ok {
			// Fall back to chat completions with conversion
			chatCmd, ok := p.impl.Commands["chat_completions"].(drivers.ChatCompletionsCommand)
			if !ok {
				continue
			}

			// Convert responses request to chat completions
			converter := &styles.DefaultConverter{}
			chatReq, err := converter.ConvertRequest(providerReq, styles.StyleOpenAIResponses, styles.StyleOpenAIChat)
			if err != nil {
				continue
			}

			if providerReq.IsStreaming() {
				// Handle streaming with conversion
				hres, stream, err := chatCmd.DoChatCompletionsStream(&p.impl, chatReq, r)
				if err != nil {
					if displayErr == nil {
						displayErr = err
					}
					continue
				}

				sseWriter := sse.NewWriter(w)
				_ = sseWriter.WriteHeartbeat("ok")

				var lastChunk formats.ManagedResponse
				for chunk := range stream {
					if chunk.RuntimeError != nil {
						_ = sseWriter.WriteError(chunk.RuntimeError.Error())
						break
					}
					// Convert response back
					converted, err := converter.ConvertResponse(chunk.Data, styles.StyleOpenAIChat, styles.StyleOpenAIResponses)
					if err == nil && converted != nil {
						lastChunk = converted
						data, _ := converted.ToJSON()
						_ = sseWriter.WriteRaw(data)
					}
				}
				_ = chain.RunStreamEnd(&p.impl, r, providerReq, hres, lastChunk)
				_ = sseWriter.WriteDone()
			} else {
				hres, res, err := chatCmd.DoChatCompletions(&p.impl, chatReq, r)
				if err != nil {
					if displayErr == nil {
						displayErr = err
					}
					continue
				}

				// Convert response back
				converted, err := converter.ConvertResponse(res, styles.StyleOpenAIChat, styles.StyleOpenAIResponses)
				if err != nil {
					if displayErr == nil {
						displayErr = err
					}
					continue
				}

				converted, _ = chain.RunAfter(&p.impl, r, providerReq, hres, converted)
				data, _ := converted.ToJSON()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(data)
			}

			w.Header().Set("X-Real-Provider-Id", name)
			w.Header().Set("X-Real-Model-Id", model)
			return nil
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
		m.logger.Error("all providers failed", zap.String("model", model), zap.Error(displayErr))
		http.Error(w, displayErr.Error(), http.StatusInternalServerError)
		return nil
	}

	return next.ServeHTTP(w, r)
}

var (
	_ caddy.Provisioner           = (*OpenAIResponsesModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*OpenAIResponsesModule)(nil)
)
