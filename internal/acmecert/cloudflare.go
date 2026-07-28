package acmecert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxCloudflareResponseBytes = 1 << 20

type cloudflareClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareRecord struct {
	ID string `json:"id"`
}

type cloudflareZoneResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  []cloudflareZone  `json:"result"`
}

type cloudflareRecordResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  cloudflareRecord  `json:"result"`
}

type cloudflareDeleteResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

func (c *cloudflareClient) createTXT(
	ctx context.Context,
	identifier string,
	name string,
	value string,
) (zoneID string, recordID string, err error) {
	zone, err := zoneName(identifier)
	if err != nil {
		return "", "", err
	}
	zoneID, err = c.findZone(ctx, zone)
	if err != nil {
		return "", "", err
	}

	payload, err := json.Marshal(map[string]any{
		"type":    "TXT",
		"name":    name,
		"content": value,
		"ttl":     120,
	})
	if err != nil {
		return "", "", err
	}
	endpoint, err := url.JoinPath(c.baseURL, "zones", zoneID, "dns_records")
	if err != nil {
		return "", "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")

	var response cloudflareRecordResponse
	if err := c.doJSON(request, &response); err != nil {
		return "", "", err
	}
	if !response.Success || response.Result.ID == "" {
		return "", "", cloudflareAPIError(response.Errors)
	}
	return zoneID, response.Result.ID, nil
}

func (c *cloudflareClient) findZone(ctx context.Context, name string) (string, error) {
	endpoint, err := url.Parse(c.baseURL + "/zones")
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("name", name)
	query.Set("status", "active")
	query.Set("per_page", "1")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)

	var response cloudflareZoneResponse
	if err := c.doJSON(request, &response); err != nil {
		return "", err
	}
	if !response.Success {
		return "", cloudflareAPIError(response.Errors)
	}
	for _, zone := range response.Result {
		if strings.EqualFold(zone.Name, name) && zone.ID != "" {
			return zone.ID, nil
		}
	}
	return "", fmt.Errorf("Cloudflare DNS zone %q was not found or is not available to this token", name)
}

func (c *cloudflareClient) deleteRecord(ctx context.Context, zoneID, recordID string) error {
	endpoint, err := url.JoinPath(c.baseURL, "zones", zoneID, "dns_records", recordID)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)

	var response cloudflareDeleteResponse
	if err := c.doJSON(request, &response); err != nil {
		return err
	}
	if !response.Success {
		return cloudflareAPIError(response.Errors)
	}
	return nil
}

func (c *cloudflareClient) doJSON(request *http.Request, target any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Cloudflare API request failed: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCloudflareResponseBytes))
	if err != nil {
		return fmt.Errorf("read Cloudflare API response: %w", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("Cloudflare API returned an invalid response (HTTP %d)", response.StatusCode)
	}
	return nil
}

func cloudflareAPIError(errors []cloudflareError) error {
	if len(errors) == 0 {
		return fmt.Errorf("Cloudflare API rejected the DNS change")
	}
	message := strings.TrimSpace(errors[0].Message)
	if message == "" {
		message = "Cloudflare API rejected the DNS change"
	}
	if errors[0].Code != 0 {
		return fmt.Errorf("Cloudflare API error %d: %s", errors[0].Code, message)
	}
	return fmt.Errorf("Cloudflare API error: %s", message)
}
