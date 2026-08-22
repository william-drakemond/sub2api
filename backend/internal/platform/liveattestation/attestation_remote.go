//go:build !darwin

package liveattestation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Sub2API can only mint a ChatGPT DeviceCheck attestation on Apple Silicon macOS,
// because the token comes from the official ChatGPT app's bundled native module.
// When Sub2API itself runs elsewhere, it can delegate to a helper on such a Mac:
// set SUB2API_ATTESTATION_REMOTE_URL to that helper's base URL.
//
// With the variable unset the behaviour is unchanged (ErrUnsupportedPlatform),
// so this stays inert for every deployment that does not opt in.
const (
	remoteEndpointEnv = "SUB2API_ATTESTATION_REMOTE_URL"
	remoteTokenEnv    = "SUB2API_ATTESTATION_REMOTE_TOKEN"
	remoteTimeout     = 5 * time.Second
	remoteMaxBytes    = 16 * 1024
)

type remoteProvider struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewProvider() Provider {
	endpoint := strings.TrimSpace(os.Getenv(remoteEndpointEnv))
	if endpoint == "" {
		return unsupportedProvider{}
	}
	return &remoteProvider{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    strings.TrimSpace(os.Getenv(remoteTokenEnv)),
		client:   &http.Client{Timeout: remoteTimeout},
	}
}

func (p *remoteProvider) Check(ctx context.Context) error {
	_, err := p.fetch(ctx, "/healthz")
	return err
}

func (p *remoteProvider) Generate(ctx context.Context) (string, error) {
	payload, err := p.fetch(ctx, "/attestation")
	if err != nil {
		return "", err
	}
	header := strings.TrimSpace(payload)
	// Mirror the darwin provider's validation so a broken helper cannot poison the call.
	if len(header) < 20 || len(header) > remoteMaxBytes || !json.Valid([]byte(header)) {
		return "", errors.New("remote DeviceCheck helper returned a malformed attestation")
	}
	return header, nil
}

func (p *remoteProvider) fetch(ctx context.Context, path string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, remoteTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.endpoint+path, nil)
	if err != nil {
		return "", fmt.Errorf("build remote DeviceCheck request: %w", err)
	}
	if p.token != "" {
		request.Header.Set("Authorization", "Bearer "+p.token)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("remote DeviceCheck helper is unreachable: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, remoteMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read remote DeviceCheck response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		reason := strings.TrimSpace(string(body))
		if len(reason) > 240 {
			reason = reason[:240]
		}
		if reason == "" {
			reason = response.Status
		}
		return "", fmt.Errorf("remote DeviceCheck helper failed: %s", reason)
	}
	return string(body), nil
}
