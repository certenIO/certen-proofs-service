package main

import (
	"strings"
	"testing"
)

// The service starts only with enforcement explicitly on, or development explicitly on. A forgotten variable, or
// enforcement off outside development, stops it at boot with a named error instead of serving the corpus open.
func TestAuthStartupMatrix(t *testing.T) {
	cases := []struct {
		name, required, dev, wantErr string
	}{
		{"enforced", "true", "", ""},
		{"enforced, dev flag false", "true", "false", ""},
		{"development, enforcement unset", "", "true", ""},
		{"development, enforcement off", "false", "true", ""},
		{"nothing set", "", "", "AUTH_NOT_EXPLICIT"},
		{"dev flag false and nothing else", "", "false", "AUTH_NOT_EXPLICIT"},
		{"enforcement off outside development", "false", "", "AUTH_DISABLED_OUTSIDE_DEVELOPMENT"},
		{"enforcement off, dev false", "false", "false", "AUTH_DISABLED_OUTSIDE_DEVELOPMENT"},
		{"enforcement garbage", "yes", "", "AUTH_DISABLED_OUTSIDE_DEVELOPMENT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AUTH_REQUIRED", c.required)
			t.Setenv("DEVELOPMENT_MODE", c.dev)
			err := authStartupCheck()
			if c.wantErr == "" && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("got %v, want %s", err, c.wantErr)
			}
		})
	}
}
