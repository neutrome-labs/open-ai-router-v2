package chatcompletionsplugins

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

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
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
var compactionCache sync.Map // map[string][]map[string]any

// summaryPrompt is the system prompt used to summarize conversations
const summaryPrompt = `You are a conversation summarizer. Your task is to create a concise but comprehensive summary of the conversation history provided.

Requirements:
- Preserve all important context, decisions, and information
- Maintain the logical flow of the conversation
- Keep technical details, code snippets references, and specific values
- The summary should allow continuing the conversation without losing context
- Format as a clear, readable summary paragraph or bullet points
- Do NOT include any preamble like "Here's a summary" - just output the summary directly`

// estimateTokens provides a rough token count estimate (avg 4 chars per token)
func estimateTokens(text string) int {
	return (len(text) + 3) / 4
}

// estimateMessagesTokens estimates total tokens in messages array
func estimateMessagesTokens(messages []any) int {
	total := 0
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if content, ok := msgMap["content"].(string); ok {
			total += estimateTokens(content)
		}
		// Add overhead for role, etc.
		total += 4
	}
	return total
}

// hashMessages creates a deterministic hash of messages for caching
func hashMessages(messages []any) string {
	data, _ := json.Marshal(messages)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // use first 16 bytes
}

// extractMessages separates system messages, compactable messages, and the last user message
func (z *Zip) extractMessages(messages []any) (system []any, compactable []any, lastInput []any, firstUser []any) {
	if len(messages) == 0 {
		return nil, nil, nil, nil
	}

	// Extract system messages from the beginning
	idx := 0
	for idx < len(messages) {
		msg, ok := messages[idx].(map[string]any)
		if !ok {
			break
		}
		if msg["role"] != "system" {
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
		msg, ok := remaining[0].(map[string]any)
		if ok && msg["role"] == "user" {
			firstUser = append(firstUser, remaining[0])
			remaining = remaining[1:]
		}
	}

	if len(remaining) == 0 {
		return system, nil, nil, firstUser
	}

	// Last message(s) to preserve - find the last user message and any following assistant response
	lastIdx := len(remaining) - 1
	lastMsg, ok := remaining[lastIdx].(map[string]any)
	if ok {
		if lastMsg["role"] == "user" {
			lastInput = []any{remaining[lastIdx]}
			compactable = remaining[:lastIdx]
		} else if lastMsg["role"] == "assistant" && lastIdx > 0 {
			// Keep last user + assistant pair
			prevMsg, ok := remaining[lastIdx-1].(map[string]any)
			if ok && prevMsg["role"] == "user" {
				lastInput = remaining[lastIdx-1:]
				compactable = remaining[:lastIdx-1]
			} else {
				lastInput = []any{remaining[lastIdx]}
				compactable = remaining[:lastIdx]
			}
		} else {
			lastInput = []any{remaining[lastIdx]}
			compactable = remaining[:lastIdx]
		}
	}

	return system, compactable, lastInput, firstUser
}

// doSummarize calls the AI to summarize the conversation
func (z *Zip) doSummarize(p *services.ProviderImpl, r *http.Request, messages []any, model string) (string, error) {
	// Build conversation text for summarization
	var convBuilder strings.Builder
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		content, _ := msgMap["content"].(string)
		convBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	summaryReq := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": summaryPrompt},
			{"role": "user", "content": "Please summarize this conversation:\n\n" + convBuilder.String()},
		},
		"max_tokens": 2048,
	}

	reqBody, err := json.Marshal(summaryReq)
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

	var result map[string]any
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("failed to parse summary response: %w", err)
	}

	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in summary response")
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("invalid content format")
	}

	return content, nil
}

func (z *Zip) Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error) {
	// Parse max tokens from params (e.g., "65535")
	maxTokens := 65535
	if params != "" {
		if parsed, err := strconv.Atoi(params); err == nil && parsed > 0 {
			maxTokens = parsed
		}
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, nil
	}

	messages, ok := req["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body, nil
	}

	// Check if we need to compact
	currentTokens := estimateMessagesTokens(messages)
	if currentTokens <= maxTokens {
		return body, nil
	}

	model, _ := req["model"].(string)
	if model == "" {
		return body, nil
	}

	// Extract message parts
	systemMsgs, compactable, lastInput, firstUser := z.extractMessages(messages)

	// Nothing to compact
	if len(compactable) == 0 {
		return body, nil
	}

	// Check cache first (unless disabled)
	var compactedMessages []any
	cacheKey := ""

	if !z.DisableCache {
		cacheKey = hashMessages(compactable)
		if cached, ok := compactionCache.Load(cacheKey); ok {
			compactedMessages = cached.([]any)
		}
	}

	// Need to generate summary
	if compactedMessages == nil {
		summary, err := z.doSummarize(p, r, compactable, model)
		if err != nil {
			// On error, return original body - don't break the request
			return body, nil
		}

		// Create compacted message
		compactedMessages = []any{
			map[string]any{
				"role":    "user",
				"content": "[Previous conversation summary]\n" + summary,
			},
			map[string]any{
				"role":    "assistant",
				"content": "I understand. I have the context from our previous conversation. Please continue.",
			},
		}

		// Cache the result
		if !z.DisableCache && cacheKey != "" {
			compactionCache.Store(cacheKey, compactedMessages)
		}
	}

	// Rebuild messages array
	newMessages := make([]any, 0, len(systemMsgs)+len(firstUser)+len(compactedMessages)+len(lastInput))
	newMessages = append(newMessages, systemMsgs...)
	newMessages = append(newMessages, firstUser...)
	newMessages = append(newMessages, compactedMessages...)
	newMessages = append(newMessages, lastInput...)

	req["messages"] = newMessages

	return json.Marshal(req)
}

func (z *Zip) After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error) {
	return res, nil
}
