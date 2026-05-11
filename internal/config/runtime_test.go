package config

import "testing"

func TestValidateProviderURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
		desc    string
	}{
		{"https://api.openai.com/v1", false, "valid https"},
		{"http://localhost:11434/v1", false, "localhost allowed (Ollama)"},
		{"http://127.0.0.1:11434/v1", false, "loopback allowed"},
		{"http://192.168.1.100:8080/v1", false, "RFC1918 allowed (LAN server)"},
		{"http://10.0.0.1/v1", false, "RFC1918 10.x allowed"},
		{"http://169.254.169.254/latest/meta-data", true, "AWS metadata blocked"},
		{"http://169.254.0.1/v1", true, "link-local blocked"},
		{"ftp://example.com/v1", true, "non-http scheme rejected"},
		{"file:///etc/passwd", true, "file scheme rejected"},
		{"not a url", true, "garbage rejected"},
		{"", true, "empty url is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validateProviderURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateProviderURL(%q) error=%v, wantErr=%v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestValidateProvidersRejectsDuplicateID(t *testing.T) {
	providers := []ProviderConfig{
		{ID: "p1", BaseURL: "https://api.openai.com/v1"},
		{ID: "P1", BaseURL: "https://api.openai.com/v1"},
	}
	if err := ValidateProviders(providers); err == nil {
		t.Fatal("expected error for duplicate provider id (case-insensitive)")
	}
}

func TestValidateProvidersRejectsBadURL(t *testing.T) {
	providers := []ProviderConfig{
		{ID: "evil", BaseURL: "http://169.254.169.254/latest/meta-data"},
	}
	if err := ValidateProviders(providers); err == nil {
		t.Fatal("expected error for link-local base_url")
	}
}
