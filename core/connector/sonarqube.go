// core/connector/sonarqube.go
package connector

import (
	"fmt"
	"net/http"
)

// SonarQubeConnector checks reachability and auth against a configured
// SonarQube server (self-hosted or SonarCloud — both expose the same
// /api/system/status shape). The token comes from core/secretstore via
// the caller (Task 9's cmdStatus); this package never touches secret
// storage directly, keeping connector and secretstore independently
// testable.
type SonarQubeConnector struct {
	baseURL, token string
	client         *http.Client
}

func NewSonarQubeConnector(baseURL, token string, client *http.Client) *SonarQubeConnector {
	if client == nil {
		client = http.DefaultClient
	}
	return &SonarQubeConnector{baseURL: baseURL, token: token, client: client}
}

func (c *SonarQubeConnector) Name() string { return "sonarqube" }

func (c *SonarQubeConnector) Phases() []Phase {
	return []Phase{
		{Name: "auth", Run: c.auth},
	}
}

func (c *SonarQubeConnector) auth() (Result, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/system/status", nil)
	if err != nil {
		return Result{}, err
	}
	req.SetBasicAuth(c.token, "")
	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return Result{}, fmt.Errorf("token rejected (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return Result{Connected: true, Detail: c.baseURL}, nil
}
