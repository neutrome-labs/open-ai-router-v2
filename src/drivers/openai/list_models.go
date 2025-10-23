package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ListModels struct {
}

func (c *ListModels) DoListModels(p *service.ProviderImpl, r *http.Request) ([]commands.ListModelsModel, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += "/models"

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")

	req := &http.Request{
		Method: "GET",
		URL:    &targetUrl,
		Header: targetHeader,
	}

	authVal, err := p.Router.AuthManager.CollectTargetAuth("list_models", p, r, req)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		req.Header.Set("Authorization", "Bearer "+authVal)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if authVal != "" {
			req.Header.Set("Authorization", authVal)
			resp, err = http.DefaultClient.Do(req)
		}
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []commands.ListModelsModel `json:"data"`
	}

	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("%s; data: %s", err, string(data))
	}

	return result.Data, nil
}
