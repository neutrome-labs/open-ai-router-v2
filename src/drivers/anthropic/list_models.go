package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// ListModels implements listing models for Anthropic APIs
type ListModels struct{}

func (c *ListModels) DoListModels(p *services.ProviderImpl, r *http.Request) ([]drivers.ListModelsModel, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += "/models"

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")

	// Add Anthropic-specific headers
	if headerName, headerVal, ok := styles.StyleRequiresVersion(styles.StyleAnthropic); ok {
		targetHeader.Set(headerName, headerVal)
	}

	req := &http.Request{
		Method: "GET",
		URL:    &targetUrl,
		Header: targetHeader,
	}
	req = req.WithContext(r.Context())

	authVal, err := p.Router.AuthManager.CollectTargetAuth("list_models", p, r, req)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		req.Header.Set(styles.StyleAuthHeader(styles.StyleAnthropic), styles.StyleAuthFormat(styles.StyleAnthropic, authVal))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Anthropic models endpoint structure
	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}

	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("%s; data: %s", err, string(data))
	}

	models := make([]drivers.ListModelsModel, len(result.Data))
	for i, m := range result.Data {
		models[i] = drivers.ListModelsModel{
			ID:   m.ID,
			Name: m.DisplayName,
		}
	}

	return models, nil
}
