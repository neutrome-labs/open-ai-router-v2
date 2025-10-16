package modules

import (
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"go.uber.org/zap"
)

// ChatCompletionsModule serves chat completions under any path.
type ChatCompletionsModule struct {
	RouterName string `json:"router,omitempty"`

	proxy  *httputil.ReverseProxy
	logger *zap.Logger
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
				return nil, h.Errf("unrecognized ai_chat_completions option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (ChatCompletionsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_chat_completions",
		New: func() caddy.Module { return new(ChatCompletionsModule) },
	}
}

func (m *ChatCompletionsModule) Provision(ctx caddy.Context) error {
	m.proxy = httputil.NewSingleHostReverseProxy(&url.URL{})
	m.logger = ctx.Logger(m)
	return nil
}

func (m *ChatCompletionsModule) serveChatCompletions(p *ProviderDef, cmd commands.ChatCompletionsCommand, req *formats.ChatCompletionsRequest, w http.ResponseWriter, r *http.Request) error {
	res, err := cmd.DoChatCompletions(&p.impl, req, r)
	if err != nil {
		m.logger.Error("chat completions error", zap.String("provider", p.Name), zap.Error(err))
		return err
	}

	data, err := res.ToJson()
	if err != nil {
		m.logger.Error("chat completions error", zap.String("provider", p.Name), zap.Error(err))
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		m.logger.Error("chat completions error", zap.String("provider", p.Name), zap.Error(err))
		return err
	}

	return nil
}

func (m *ChatCompletionsModule) serveChatCompletionsStream(p *ProviderDef, cmd commands.ChatCompletionsCommand, req *formats.ChatCompletionsRequest, w http.ResponseWriter, r *http.Request) error {
	flusher, _ := w.(http.Flusher)

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	hdr.Del("Content-Encoding")

	_, _ = w.Write([]byte(":ok\n\n"))
	if flusher != nil {
		flusher.Flush()
	}

	stream, err := cmd.DoChatCompletionsStream(&p.impl, req, r)
	if err != nil {
		m.logger.Error("chat completions stream error (start)", zap.String("provider", p.Name), zap.Error(err))
		_, _ = w.Write([]byte("data: {\"error\":\"start failed\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		return err
	}

	for chunk := range stream {
		if chunk.RuntimeError != nil {
			_, _ = w.Write([]byte("data: {\"error\":\"" + chunk.RuntimeError.Error() + "\"}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			return nil
		}

		data, err := chunk.ToJson()
		if err != nil {
			m.logger.Error("chat completions stream error", zap.String("provider", p.Name), zap.Error(err))
			_, _ = w.Write([]byte("data: {\"error\":\"marshal failed\"}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			return err
		}

		if _, err := w.Write([]byte("data: ")); err != nil {
			m.logger.Error("chat completions stream write error", zap.String("provider", p.Name), zap.Error(err))
			return err
		}
		if _, err := w.Write(data); err != nil {
			m.logger.Error("chat completions stream write error", zap.String("provider", p.Name), zap.Error(err))
			return err
		}
		if _, err := w.Write([]byte("\n\n")); err != nil {
			m.logger.Error("chat completions stream write error", zap.String("provider", p.Name), zap.Error(err))
			return err
		}

		if flusher != nil {
			flusher.Flush()
		}
	}

	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func (m *ChatCompletionsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return nil
	}

	req := formats.ChatCompletionsRequest{}
	if err := req.FromJson(reqBody); err != nil {
		m.logger.Error("failed to parse request body", zap.Error(err))
		http.Error(w, "failed to parse request body", http.StatusBadRequest)
		return nil
	}

	router, ok := GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("RouterName not found", zap.String("name", m.RouterName))
		http.Error(w, "RouterName not found", http.StatusInternalServerError)
		return nil
	}

	providers, model := router.ResolveProvidersOrderAndModel(req.Model)

	var displayErr error
	for _, name := range providers {
		p, ok := router.Providers[name]
		if !ok {
			m.logger.Error("provider not found", zap.String("name", name))
			continue
		}

		if _, ok := p.impl.Commands["chat_completions"]; !ok {
			continue
		}

		cmd, ok := p.impl.Commands["chat_completions"].(commands.ChatCompletionsCommand)
		if !ok {
			continue
		}

		if req.Stream {
			err = m.serveChatCompletionsStream(p, cmd, &req, w, r)
			if err != nil {
				if displayErr == nil {
					displayErr = err
				}
				continue
			}
		} else {
			err = m.serveChatCompletions(p, cmd, &req, w, r)
			if err != nil {
				if displayErr == nil {
					displayErr = err
				}
				continue
			}
		}
	}

	if displayErr != nil {
		m.logger.Error("all providers failed", zap.String("model", model), zap.Error(displayErr))
		http.Error(w, displayErr.Error(), http.StatusInternalServerError)
		return nil
	}

	return next.ServeHTTP(w, r)
}

var (
	_ caddy.Provisioner           = (*ChatCompletionsModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*ChatCompletionsModule)(nil)
)
