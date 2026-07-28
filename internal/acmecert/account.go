package acmecert

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/crypto/acme"
)

type zeroSSLEABResponse struct {
	Success bool   `json:"success"`
	KID     string `json:"eab_kid"`
	HMACKey string `json:"eab_hmac_key"`
	Error   *struct {
		Code int    `json:"code"`
		Type string `json:"type"`
	} `json:"error"`
}

func (m *Manager) registerAccount(
	ctx context.Context,
	client *acme.Client,
	ca string,
	email string,
	accountKeyCreated bool,
) error {
	account := &acme.Account{Contact: []string{"mailto:" + email}}
	if ca == CAZeroSSL && accountKeyCreated {
		binding, err := m.generateZeroSSLEAB(ctx, email)
		if err != nil {
			return err
		}
		account.ExternalAccountBinding = binding
	}

	if _, err := client.Register(ctx, account, acme.AcceptTOS); err == nil ||
		errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil
	} else if ca != CAZeroSSL || accountKeyCreated {
		return fmt.Errorf("register ACME account: %w", err)
	}

	binding, err := m.generateZeroSSLEAB(ctx, email)
	if err != nil {
		return err
	}
	account.ExternalAccountBinding = binding
	if _, err := client.Register(ctx, account, acme.AcceptTOS); err != nil &&
		!errors.Is(err, acme.ErrAccountAlreadyExists) {
		return fmt.Errorf("register ZeroSSL ACME account: %w", err)
	}
	return nil
}

func (m *Manager) generateZeroSSLEAB(
	ctx context.Context,
	email string,
) (*acme.ExternalAccountBinding, error) {
	form := url.Values{"email": []string{email}}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		m.zeroSSLEABURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := m.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request ZeroSSL EAB credentials: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read ZeroSSL EAB response: %w", err)
	}

	var payload zeroSSLEABResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("ZeroSSL returned an invalid EAB response (HTTP %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !payload.Success {
		if payload.Error != nil {
			return nil, fmt.Errorf(
				"ZeroSSL EAB error %d: %s",
				payload.Error.Code,
				payload.Error.Type,
			)
		}
		return nil, fmt.Errorf("ZeroSSL EAB request failed with HTTP %d", response.StatusCode)
	}
	if payload.KID == "" || payload.HMACKey == "" {
		return nil, fmt.Errorf("ZeroSSL returned incomplete EAB credentials")
	}
	key, err := decodeBase64URL(payload.HMACKey)
	if err != nil {
		return nil, fmt.Errorf("decode ZeroSSL EAB key: %w", err)
	}
	return &acme.ExternalAccountBinding{KID: payload.KID, Key: key}, nil
}

func decodeBase64URL(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}
