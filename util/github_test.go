package util

import (
	"os"
	"testing"
)

func TestGetGHTokenFromEnv(t *testing.T) {
	const want = "my-token-123"
	os.Setenv(GITHUB_TOKEN_ENV, want)
	defer os.Unsetenv(GITHUB_TOKEN_ENV)

	got := GetGHToken()
	if got != want {
		t.Fatalf("expected token %q, got %q", want, got)
	}
}
