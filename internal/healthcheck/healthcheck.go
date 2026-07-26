package healthcheck

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const requestTimeout = 5 * time.Second

// Check probes the panel's fixed local health endpoint over HTTP and HTTPS.
// Trying both schemes keeps the container health command valid when the
// administrator enables or removes a direct TLS certificate.
func Check(ctx context.Context, address string) error {
	client := &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // local endpoint; certificate identity is irrelevant
		},
	}

	var attempts []error
	for _, scheme := range []string{"http", "https"} {
		endpoint := scheme + "://" + address + "/healthz"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			attempts = append(attempts, err)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			attempts = append(attempts, fmt.Errorf("%s: %w", scheme, err))
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		attempts = append(attempts, fmt.Errorf("%s: status %d", scheme, resp.StatusCode))
	}
	return errors.Join(attempts...)
}
