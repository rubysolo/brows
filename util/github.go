package util

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

const GITHUB_TOKEN_ENV = "GITHUB_OAUTH_TOKEN"
const GH_CLI_COMMAND = "gh auth token"

// GetToken retrieves a GitHub access token from the environment variable `GITHUB_OAUTH_TOKEN`.
// If the environment variable is not set, it attempts to obtain the token by executing
// the GitHub CLI `gh auth token` command. If both methods fail, the function terminates
// the program with a fatal error, providing guidance for the user.
func GetGHToken() string {
	token := os.Getenv(GITHUB_TOKEN_ENV)
	if token != "" {
		return token
	}


	command_arr := strings.Split(GH_CLI_COMMAND, " ")
	out, err := exec.Command(command_arr[0], command_arr[1:]...).Output()
	if err != nil {
		log.Fatal("GITHUB_OAUTH_TOKEN not set and failed to retrieve token from gh CLI: ", err, "\nPlease set GITHUB_OAUTH_TOKEN or run 'gh auth login'")
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		log.Fatal("GITHUB_OAUTH_TOKEN not set and gh CLI did not return a token.\nPlease set GITHUB_OAUTH_TOKEN or run 'gh auth login'")
	}
	return trimmed
}
