// Package tools implements the API to other tools like Helmfile, or Smallstep's 'step' CLI
// Initially I wanted to compile in the direct sources from their respective Go modules
//
// However, Bazel struggles with compilation of the dependencies, mainly due to issues with the
// new 'bzlmod' dependency system
package tools

import (
	"encoding/json"
	"fmt"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"strings"

	"github.com/go-resty/resty/v2"
)

// Executables represents and executables that either a Tool or another required programs depends on
type Executable int

const (
	kubectl Executable = iota
	helm
	helmfile
	stepCA
	kustomize
)

// String implements the fmt.Stringer interface for the new Executable type
func (e Executable) String() string {
	return [...]string{"kubectl", "helm", "helmfile", "step-ca", "kustomize"}[e]
}

// Index makes the current Executable index retrievable
func (e Executable) Index() int {
	return int(e)
}

var (
	githubURL   = "https://github.com/"
	executables = []Executable{kubectl, helm, helmfile, stepCA, kustomize}
)

// Find checks the system for the required executables. It returns the first error that occurs during
// the search, thereby setting the map return value to nil.
func Find() (map[Executable]string, error) {
	t := make(map[Executable]string)

	for _, v := range executables {
		path, err := proc.LookPath(v.String())
		if err != nil {
			return nil, fmt.Errorf("could not find tool: %s. error: %v", v, err)
		}

		t[v] = path
	}

	return t, nil
}

// releaseResponse represents a GitHub API response
type releaseResponse struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

// latestGitHubRelease fetches the latest release data for a given GitHub Repository. If an authenticated request
// should be made, the passed in client should have a Bearer Auth Token set for every request. To do so, call the
// client.SetAuthToken method before passing it in
func latestGitHubRelease(client *resty.Client, url string) (*releaseResponse, error) {
	req := client.R().
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28")

	owner, repo, err := parseGitHubURL(url)
	if err != nil {
		return nil, err
	}

	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	resp, err := req.Get(api)
	if err != nil {
		return nil, err
	}

	d := &releaseResponse{}
	if err := json.Unmarshal(resp.Body(), d); err != nil {
		return nil, err
	}

	return d, nil
}

// parseGitHubURL parses the repository owner and name from the GitHub URL
// If a non-GitHub URL is given, the function will return an error.
func parseGitHubURL(url string) (string, string, error) {
	var s string

	if strings.Contains(url, githubURL) {
		s = strings.TrimPrefix(url, githubURL)
	} else {
		return "", "", fmt.Errorf("cannot parse GitHub URL for non-GitHub-URL: %s", url)
	}

	ss := strings.Split(s, "/")
	return ss[0], ss[1], nil
}

// Trim the v from the GitHub Tag
func versionFromTag(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// Add a 'v' to a SemVer version if needed
func tagFromVersion(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}

	return fmt.Sprintf("v%s", version)
}
