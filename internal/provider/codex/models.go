package codex

import (
	"context"
	"errors"

	"github.com/liyiligang/loom/internal/provider"
)

func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	models := []provider.Model{}
	cursor := ""
	for page := 0; page < 10; page++ {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result struct {
			Data []struct {
				Model       string `json:"model"`
				DisplayName string `json:"displayName"`
				IsDefault   bool   `json:"isDefault"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := c.Call(ctx, "model/list", params, &result); err != nil {
			return nil, err
		}
		for _, value := range result.Data {
			if value.Model != "" {
				models = append(models, provider.Model{ID: value.Model, Name: value.DisplayName, IsDefault: value.IsDefault})
			}
		}
		if result.NextCursor == nil || *result.NextCursor == "" || *result.NextCursor == cursor {
			break
		}
		cursor = *result.NextCursor
	}
	return models, nil
}

func (c *Client) selectedModel(ctx context.Context, cwd, selected string) (string, error) {
	if selected != "" {
		return selected, nil
	}
	// A blank turn override would retain the previous model. Resolve the agent's
	// configured default explicitly when returning from a scope override.
	var config struct {
		Config struct {
			Model string `json:"model"`
		} `json:"config"`
	}
	if err := c.Call(ctx, "config/read", map[string]any{"cwd": cwd, "includeLayers": false}, &config); err == nil && config.Config.Model != "" {
		return config.Config.Model, nil
	}
	models, err := c.ListModels(ctx)
	if err != nil {
		return "", err
	}
	for _, value := range models {
		if value.IsDefault {
			return value.ID, nil
		}
	}
	return "", errors.New("The agent did not report a default model")
}
