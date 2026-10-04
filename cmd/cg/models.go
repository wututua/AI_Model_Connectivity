package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cg/internal/config"
	"cg/internal/provider"
)

func (a *application) DiscoverModels(ctx context.Context, query config.ModelDiscoveryRequest) ([]string, error) {
	current := a.currentConfig()
	var existing config.ProviderConfig
	if query.ProviderID != "" {
		found := false
		for _, item := range current.Providers {
			if item.ID == query.ProviderID {
				existing, found = item, true
				break
			}
		}
		if !found {
			return nil, errors.New("Provider 不存在，请刷新后重试")
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(query.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("请先填写 Base URL")
	}
	// A draft URL must not silently send an existing secret to a different endpoint.
	if existing.APIKey != "" && query.APIKey == "" && !query.ClearAPIKey &&
		baseURL != strings.TrimRight(strings.TrimSpace(existing.BaseURL), "/") {
		return nil, errors.New("Base URL 已变更，请重新填写 API Key 后同步模型")
	}
	draft := config.ApplyProviderUpdate(existing, config.ProviderUpdate{
		ID: "model-discovery", Type: query.Type, BaseURL: baseURL,
		APIKey: query.APIKey, ClearAPIKey: query.ClearAPIKey,
	})
	if err := config.ValidateProviders([]config.ProviderConfig{draft}); err != nil {
		return nil, err
	}
	timeout := time.Duration(current.ModelListTimeoutSeconds * float64(time.Second))
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := provider.New(draft)
	if closer, ok := client.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	models, err := client.Models(ctx)
	if err != nil {
		return nil, fmt.Errorf("同步模型失败: %w", err)
	}
	if models == nil {
		models = []string{}
	}
	return models, nil
}
