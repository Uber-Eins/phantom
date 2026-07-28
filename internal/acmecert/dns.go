package acmecert

import (
	"context"
	"fmt"
	"net"
	"slices"
	"time"
)

const (
	dnsPropagationTimeout = 2 * time.Minute
	dnsPollingInterval    = 2 * time.Second
)

func waitForTXTRecord(ctx context.Context, name, value string) error {
	waitCtx, cancel := context.WithTimeout(ctx, dnsPropagationTimeout)
	defer cancel()

	ticker := time.NewTicker(dnsPollingInterval)
	defer ticker.Stop()

	for {
		records, err := net.DefaultResolver.LookupTXT(waitCtx, name)
		if err == nil && slices.Contains(records, value) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("DNS TXT record %q did not propagate within %s", name, dnsPropagationTimeout)
		case <-ticker.C:
		}
	}
}
