package plugins

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"go.uber.org/zap"
)

// Zip implements auto-compaction for long conversations.
// Usage variants:
//   - zip:65535         - compact when context > 65535 tokens (with cache)
//   - zipc:65535        - same but preserve first user message
//   - zips:65535        - same but disable cache (recompact each time)
//   - zipsc:65535       - preserve first + no cache
type Zip struct {
	PreserveFirst bool // preserve first user message in compaction
	DisableCache  bool // disable caching of compacted conversations
}

// compactionCache stores mapping from original messages hash to compacted messages
var compactionCache sync.Map // map[string][]formats.Message

// summaryPrompt is the system prompt used to summarize conversations
const summaryPrompt = `You are a conversation summarizer. Your task is to create a concise but comprehensive summary of the conversation history provided.

Requirements:
- Preserve all important context, decisions, and information
- Maintain the logical flow of the conversation
- Keep technical details, code snippets references, and specific values
- The summary should allow continuing the conversation without losing context
- Format as a clear, readable summary paragraph or bullet points
- Do NOT include any preamble like "Here's a summary" - just output the summary directly`

func (z *Zip) Name() string {
	if z.PreserveFirst && z.DisableCache {
		return "zipsc"
	} else if z.PreserveFirst {
		return "zipc"
	} else if z.DisableCache {
		return "zips"
	}
	return "zip"
}

// estimateTokens provides a rough token count estimate (avg 4 chars per token)
func estimateTokens(text string) int {
	return (len(text) + 3) / 4
}

// estimateMessagesTokens estimates total tokens in messages array
func estimateMessagesTokens(messages []formats.Message) int {
	total := 0
	for _, msg := range messages {
		if content, ok := msg.Content.(string); ok {
			total += estimateTokens(content)
		}
		// Add overhead for role, etc.
		total += 4
	}
	return total
}

// hashMessages creates a deterministic hash of messages for caching
func hashMessages(messages []formats.Message) string {
	data, _ := json.Marshal(messages)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // use first 16 bytes
}

// extractMessages separates system messages, compactable messages, and the last user message
func (z *Zip) extractMessages(messages []formats.Message) (system []formats.Message, compactable []formats.Message, lastInput []formats.Message, firstUser []formats.Message) {
	if len(messages) == 0 {
		return nil, nil, nil, nil
	}

	// Extract system messages from the beginning
	idx := 0
	for idx < len(messages) {
		if messages[idx].Role != "system" {
			break
		}
		system = append(system, messages[idx])
		idx++
	}

	remaining := messages[idx:]
	if len(remaining) == 0 {
		return system, nil, nil, nil
	}

	// Extract first user message if PreserveFirst is set
	if z.PreserveFirst && len(remaining) > 0 {
		if remaining[0].Role == "user" {
			firstUser = append(firstUser, remaining[0])
			remaining = remaining[1:]
		}
	}

	if len(remaining) == 0 {
		return system, nil, nil, firstUser
	}

	// Last message(s) to preserve - find the last user message and any following assistant response
	lastIdx := len(remaining) - 1
	lastMsg := remaining[lastIdx]

	if lastMsg.Role == "user" {
		lastInput = []formats.Message{remaining[lastIdx]}
		compactable = remaining[:lastIdx]
	} else if lastMsg.Role == "assistant" && lastIdx > 0 {
		// Keep last user + assistant pair
		prevMsg := remaining[lastIdx-1]
		if prevMsg.Role == "user" {
			lastInput = remaining[lastIdx-1:]
			compactable = remaining[:lastIdx-1]
		} else {
			lastInput = []formats.Message{remaining[lastIdx]}
			compactable = remaining[:lastIdx]
		}
	} else {
		lastInput = []formats.Message{remaining[lastIdx]}
		compactable = remaining[:lastIdx]
	}

	return system, compactable, lastInput, firstUser
}

// doSummarize calls the AI to summarize the conversation
func (z *Zip) doSummarize(p *services.ProviderImpl, r *http.Request, messages []formats.Message, model string) (string, error) {
	// Build conversation text for summarization
	var convBuilder strings.Builder
	for _, msg := range messages {
		content, _ := msg.Content.(string)
		convBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, content))
	}

	summaryReq := &formats.OpenAIChatRequest{
		Model: model,
		Messages: []formats.Message{
			{Role: "system", Content: summaryPrompt},
			{Role: "user", Content: "Please summarize this conversation:\n\n" + convBuilder.String()},
		},
		MaxTokens: 2048,
	}

	reqBody, err := summaryReq.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal summary request: %w", err)
	}

	// Create the summarization request
	targetUrl := p.ParsedURL
	targetUrl.Path += "/chat/completions"

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")
	targetHeader.Set("Content-Type", "application/json")

	req := &http.Request{
		Method: "POST",
		URL:    &targetUrl,
		Header: targetHeader,
		Body:   io.NopCloser(bytes.NewReader(reqBody)),
	}
	req = req.WithContext(r.Context())

	authVal, err := p.Router.AuthManager.CollectTargetAuth("chat_completions", p, r, req)
	if err != nil {
		return "", fmt.Errorf("failed to get auth: %w", err)
	}
	if authVal != "" {
		req.Header.Set("Authorization", "Bearer "+authVal)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("summarization request failed: %w", err)
	}
	defer res.Body.Close()

	respData, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return "", fmt.Errorf("summarization failed: %s", string(respData))
	}

	var result formats.OpenAIChatResponse
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("failed to parse summary response: %w", err)
	}

	choices := result.GetChoices()
	if len(choices) == 0 {
		return "", fmt.Errorf("no choices in summary response")
	}

	content, _ := choices[0].Message.Content.(string)
	if content == "" {
		return "", fmt.Errorf("empty content in summary response")
	}

	return content, nil
}

func (z *Zip) Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	// Zip requires a provider to call summarization - skip if not provided
	if p == nil {
		if Logger != nil {
			Logger.Debug("zip plugin skipped - no provider context")
		}
		return req, nil
	}

	if Logger != nil {
		Logger.Debug("zip plugin before hook", zap.String("params", params))
	}

	// Parse max tokens from params (e.g., "65535")
	maxTokens := 65535
	if params != "" {
		if parsed, err := strconv.Atoi(params); err == nil && parsed > 0 {
			maxTokens = parsed
		}
	}
	if Logger != nil {
		Logger.Debug("zip max tokens configured", zap.Int("maxTokens", maxTokens))
	}

	messages := req.GetMessages()
	if len(messages) == 0 {
		if Logger != nil {
			Logger.Debug("zip no messages found, skipping")
		}
		return req, nil
	}

	// Check if we need to compact
	currentTokens := estimateMessagesTokens(messages)
	if Logger != nil {
		Logger.Debug("zip message stats",
			zap.Int("messageCount", len(messages)),
			zap.Int("estimatedTokens", currentTokens))
	}

	if currentTokens <= maxTokens {
		if Logger != nil {
			Logger.Debug("zip under token limit, no compaction needed")
		}
		return req, nil
	}

	model := req.GetModel()
	if model == "" {
		if Logger != nil {
			Logger.Debug("zip no model specified, skipping")
		}
		return req, nil
	}
	if Logger != nil {
		Logger.Debug("zip using model", zap.String("model", model))
	}

	// Extract message parts
	systemMsgs, compactable, lastInput, firstUser := z.extractMessages(messages)
	if Logger != nil {
		Logger.Debug("zip extracted messages",
			zap.Int("system", len(systemMsgs)),
			zap.Int("compactable", len(compactable)),
			zap.Int("lastInput", len(lastInput)),
			zap.Int("firstUser", len(firstUser)))
	}

	// Nothing to compact
	if len(compactable) == 0 {
		if Logger != nil {
			Logger.Debug("zip nothing to compact")
		}
		return req, nil
	}

	// Check cache first (unless disabled)
	var compactedMessages []formats.Message
	cacheKey := ""

	if !z.DisableCache {
		cacheKey = hashMessages(compactable)
		if Logger != nil {
			Logger.Debug("zip cache lookup", zap.String("cacheKey", cacheKey))
		}
		if cached, ok := compactionCache.Load(cacheKey); ok {
			if Logger != nil {
				Logger.Debug("zip cache HIT")
			}
			compactedMessages = cached.([]formats.Message)
		} else {
			if Logger != nil {
				Logger.Debug("zip cache MISS")
			}
		}
	} else {
		if Logger != nil {
			Logger.Debug("zip cache disabled")
		}
	}

	// Need to generate summary
	if compactedMessages == nil {
		if Logger != nil {
			Logger.Debug("zip generating summary")
		}
		summary, err := z.doSummarize(p, r, compactable, model)
		if err != nil {
			if Logger != nil {
				Logger.Debug("zip summarization failed", zap.Error(err))
			}
			// On error, return original body - don't break the request
			return req, nil
		}
		if Logger != nil {
			Logger.Debug("zip summary generated",
				zap.Int("summaryLength", len(summary)),
				zap.String("preview", truncateString(summary, 200)))
		}

		// Create compacted message
		compactedMessages = []formats.Message{
			{
				Role:    "user",
				Content: "[Previous conversation summary]\n" + summary,
			},
			{
				Role:    "assistant",
				Content: "I understand. I have the context from our previous conversation. Please continue.",
			},
		}

		// Cache the result
		if !z.DisableCache && cacheKey != "" {
			if Logger != nil {
				Logger.Debug("zip storing in cache")
			}
			compactionCache.Store(cacheKey, compactedMessages)
		}
	}

	// Rebuild messages array
	newMessages := make([]formats.Message, 0, len(systemMsgs)+len(firstUser)+len(compactedMessages)+len(lastInput))
	newMessages = append(newMessages, systemMsgs...)
	newMessages = append(newMessages, firstUser...)
	newMessages = append(newMessages, compactedMessages...)
	newMessages = append(newMessages, lastInput...)

	newTokens := estimateMessagesTokens(newMessages)
	if Logger != nil {
		Logger.Debug("zip compaction complete",
			zap.Int("oldMessages", len(messages)),
			zap.Int("newMessages", len(newMessages)),
			zap.Int("oldTokens", currentTokens),
			zap.Int("newTokens", newTokens))
	}

	req.SetMessages(newMessages)

	return req, nil
}

// truncateString truncates a string to maxLen chars, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (z *Zip) After(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error) {
	return res, nil
}

func (z *Zip) AfterChunk(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error) {
	return chunk, nil
}

func (z *Zip) StreamEnd(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error {
	return nil
}
