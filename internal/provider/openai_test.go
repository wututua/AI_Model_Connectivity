package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/config"
)

func TestErrorResponsesAndCredentialRedaction(t *testing.T) {
	for _, status := range []int{200, 302, 401, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(status)
			writer.Write([]byte(`{"error":{"message":"invalid key secret-api-key"},"choices":[{"message":{"content":"ignored"}}]}`))
		}))
		provider := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, APIKey: "secret-api-key"})
		_, err := provider.Models(context.Background())
		if err == nil || strings.Contains(err.Error(), "secret-api-key") {
			t.Errorf("unsafe models error: %v", err)
		}
		_, _, err = provider.Chat(context.Background(), "test", "", "test")
		if err == nil || strings.Contains(err.Error(), "secret-api-key") {
			t.Errorf("unsafe chat error: %v", err)
		}
		provider.CloseIdleConnections()
		server.Close()
	}
}
