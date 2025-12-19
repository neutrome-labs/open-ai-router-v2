package styles

import (
	"encoding/json"
	"fmt"
)

// ================================================================================
// Conversion Functions between Chat Completions and Responses APIs
// ================================================================================

// ConvertChatCompletionsRequestToResponses converts a Chat Completions request to Responses format
func ConvertChatCompletionsRequestToResponses(reqBody []byte) ([]byte, error) {
	chatReq, err := ParseChatCompletionsRequest(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to unmarshal chat completions request: %w", err)
	}

	responsesReq := &ResponsesRequest{
		Model:           chatReq.Model,
		Input:           chatReq.Messages,
		Stream:          chatReq.Stream,
		MaxOutputTokens: chatReq.MaxTokens,
		Temperature:     chatReq.Temperature,
		TopP:            chatReq.TopP,
		ToolChoice:      chatReq.ToolChoice,
	}

	// Convert tools from Chat Completions format to Responses format
	for _, tool := range chatReq.Tools {
		respTool := ResponsesTool{
			Type: tool.Type,
		}
		if tool.Function != nil {
			respTool.Name = tool.Function.Name
			respTool.Description = tool.Function.Description
			respTool.Parameters = tool.Function.Parameters
			respTool.Strict = tool.Function.Strict
		}
		responsesReq.Tools = append(responsesReq.Tools, respTool)
	}

	responsesReqBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to marshal responses request: %w", err)
	}

	return responsesReqBody, nil
}

// ConvertResponsesResponseToChatCompletions converts a Responses API response to Chat Completions format
func ConvertResponsesResponseToChatCompletions(respBody []byte) ([]byte, error) {
	return respBody, nil // Passthrough for now - TODO: implement proper conversion
}

// ConvertResponsesResponseChunkToChatCompletions converts a Responses API streaming chunk to Chat Completions format
func ConvertResponsesResponseChunkToChatCompletions(chunkBody []byte) ([]byte, error) {
	return chunkBody, nil // Passthrough for now - TODO: implement proper conversion
}
