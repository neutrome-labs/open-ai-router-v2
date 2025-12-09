package modules

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/google/uuid"
	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	ccp "github.com/neutrome-labs/open-ai-router-v2/src/modules/chat_completions_plugins"
	"go.uber.org/zap"
)

// ChatCompletionsModule serves chat completions under any path.
type ChatCompletionsModule struct {
	RouterName       string `json:"router,omitempty"`
	plugins          map[string]ccp.ChatCompletionsPlugin
	mandatoryPlugins [][2]string

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

func (*ChatCompletionsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_chat_completions",
		New: func() caddy.Module { return new(ChatCompletionsModule) },
	}
}

func (m *ChatCompletionsModule) Provision(ctx caddy.Context) error {
	m.plugins = map[string]ccp.ChatCompletionsPlugin{
		"posthog": &ccp.Posthog{}, // observability
		"models":  &ccp.Models{},  // model name mapping
		"fuzz":    &ccp.Fuzz{},    // fuzzy search for model name
		/*"zip":     &ccp.Zip{},      // zip(max_context_len)
		"zipc":    &ccp.Zip{},      // zip with caption (preserve first)
		"zips":    &ccp.Zip{},      // zip with summary (summarize + last2)
		"ai18n":   &ccp.AI18n{},    // auto-translate input and output to/from english
		"optim":   &ccp.Optimize{}, // optimize first prompt for model
		"codemode":   &ccp.TSTools{},  // call tools in a mcp -> .ts way*/
	}
	m.mandatoryPlugins = [][2]string{
		{"posthog", ""},
		{"models", ""},
	}

	m.proxy = httputil.NewSingleHostReverseProxy(&url.URL{})
	m.logger = ctx.Logger(m)
	return nil
}

func (m *ChatCompletionsModule) resolvePlugins(r *http.Request, reqJson map[string]any) [][2]string {
	// Pre-allocate with capacity for mandatory plugins + estimated extras
	result := make([][2]string, len(m.mandatoryPlugins), len(m.mandatoryPlugins)+4)
	copy(result, m.mandatoryPlugins)

	// plugins from path eg plugin1:arg1/plugin2:arg2 (path already stripped of /chat/completions)
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" {
		pathParts := strings.Split(path, "/")
		for _, part := range pathParts {
			if part == "" {
				continue
			}
			if idx := strings.IndexByte(part, ':'); idx > 0 {
				result = append(result, [2]string{part[:idx], part[idx+1:]})
			} else if part != "" {
				result = append(result, [2]string{part, ""})
			}
		}
	}

	// plugin from model tags eg model="gpt-4+plugin1:arg1+plugin2:arg2"
	modelStr, _ := reqJson["model"].(string)
	if modelStr != "" {
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
						result = append(result, [2]string{name, part[colonIdx+1:]})
					}
				} else {
					result = append(result, [2]string{part, ""})
				}
			}
		}
	}

	return result
}

func (m *ChatCompletionsModule) serveChatCompletions(p *ProviderDef, cmd commands.ChatCompletionsCommand, body []byte, plugins [][2]string, w http.ResponseWriter, r *http.Request) error {
	hres, res, err := cmd.DoChatCompletions(&p.impl, body, r)
	if err != nil {
		m.logger.Error("chat completions error", zap.String("provider", p.Name), zap.Error(err))
		return err
	}

	for _, plugin := range plugins {
		pluginImpl := m.plugins[plugin[0]]
		res, err = pluginImpl.After(plugin[1], &p.impl, r, body, hres, res)
		if err != nil {
			m.logger.Error("plugin after hook error", zap.String("name", plugin[0]), zap.Error(err))
			http.Error(w, "plugin after hook error: "+plugin[0], http.StatusInternalServerError)
			return nil
		}
	}

	data, err := json.Marshal(res)
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

func (m *ChatCompletionsModule) serveChatCompletionsStream(p *ProviderDef, cmd commands.ChatCompletionsCommand, body []byte, plugins [][2]string, w http.ResponseWriter, r *http.Request) error {
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

	hres, stream, err := cmd.DoChatCompletionsStream(&p.impl, body, r)
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

		for _, plugin := range plugins {
			pluginImpl := m.plugins[plugin[0]]
			chunk.Data, err = pluginImpl.After(plugin[1], &p.impl, r, body, hres, chunk.Data)
			if err != nil {
				m.logger.Error("plugin after hook error", zap.String("name", plugin[0]), zap.Error(err))
				http.Error(w, "plugin after hook error: "+plugin[0], http.StatusInternalServerError)
				return nil
			}
		}

		data, err := json.Marshal(chunk.Data)
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

	var reqJson map[string]any
	if err = json.Unmarshal(reqBody, &reqJson); err != nil {
		m.logger.Error("failed to parse request body", zap.Error(err))
		http.Error(w, "failed to parse request body", http.StatusBadRequest)
		return nil
	}

	// todo: validate required fields: model, messages

	router, ok := GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("RouterName not found", zap.String("name", m.RouterName))
		http.Error(w, "RouterName not found", http.StatusInternalServerError)
		return nil
	}

	plugins := m.resolvePlugins(r, reqJson)
	providers, model := router.ResolveProvidersOrderAndModel(reqJson["model"].(string))
	reqJson["model"] = model // Update model name after resolving plugins

	traceId := uuid.New().String()
	r = r.WithContext(context.WithValue(r.Context(), "trace_id", traceId))

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

		xreqBody, err := json.Marshal(reqJson)
		if err != nil {
			m.logger.Error("failed to marshal xrequest body", zap.Error(err))
			http.Error(w, "failed to marshal xrequest body", http.StatusInternalServerError)
			return nil
		}

		for _, plugin := range plugins {
			pluginImpl, ok := m.plugins[plugin[0]]
			if !ok {
				m.logger.Error("plugin not found", zap.String("name", plugin[0]))
				http.Error(w, "plugin not found: "+plugin[0], http.StatusBadRequest)
				return nil
			}

			xreqBody, err = pluginImpl.Before(plugin[1], &p.impl, r, xreqBody)
			if err != nil {
				m.logger.Error("plugin before hook error", zap.String("name", plugin[0]), zap.Error(err))
				http.Error(w, "plugin before hook error: "+plugin[0], http.StatusInternalServerError)
				return nil
			}
		}

		var xreqJson map[string]any
		if err := json.Unmarshal(xreqBody, &xreqJson); err != nil {
			m.logger.Error("failed to parse request body after plugins applied", zap.Error(err))
			http.Error(w, "failed to parse request body after plugins applied", http.StatusBadRequest)
			return nil
		}

		if xreqJson["stream"] == true {
			err = m.serveChatCompletionsStream(p, cmd, xreqBody, plugins, w, r)
			if err != nil {
				if displayErr == nil {
					displayErr = err
				}
				continue
			}
		} else {
			err = m.serveChatCompletions(p, cmd, xreqBody, plugins, w, r)
			if err != nil {
				if displayErr == nil {
					displayErr = err
				}
				continue
			}
		}

		break
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
