package share

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type LinkProvider struct{}

func NewLinkProvider() *LinkProvider {
	return &LinkProvider{}
}

func (p *LinkProvider) build(host string) *LinkService {
	svc := NewLinkService()
	svc.PrepareForRequest(host)
	return svc
}

func (p *LinkProvider) LinksForClient(host string, inbound *model.Inbound, email string) []string {
	svc := p.build(host)
	svc.projectThroughFallbackMaster(inbound)
	return splitLinkLines(svc.GetLink(inbound, email))
}

func splitLinkLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
