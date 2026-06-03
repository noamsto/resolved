package gitctx

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var remoteRe = regexp.MustCompile(`github\.com[:/]([\w.-]+)/([\w.-]+?)(?:\.git)?/?$`)

func parseRemoteURL(url string) (owner, repo string, err error) {
	m := remoteRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", "", fmt.Errorf("not a github remote: %s", url)
	}
	return m[1], m[2], nil
}

// OriginRepo returns the owner/repo of the `origin` remote for the git repo
// containing dir. Returns an error if there is no github origin.
func OriginRepo(dir string) (owner, repo string, err error) {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return parseRemoteURL(string(out))
}
