package config

import (
	"strings"
	"testing"
)

func TestValidateToken(t *testing.T) {
	for _, token := range []string{"short", "contains spaces here", "line\nbreak-token-value", strings.Repeat("a", 257), strings.Repeat("\u4e2d", 16)} {
		if err := ValidateToken(token); err == nil {
			t.Errorf("accepted invalid token %q", token)
		}
	}
	if err := ValidateToken("valid-admin-token"); err != nil {
		t.Fatal(err)
	}
}
