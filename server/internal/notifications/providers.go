package notifications

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Router struct {
	APNs *APNSProvider
	FCM  *FCMProvider
}

func (r Router) Configured() bool {
	return r.APNs != nil || r.FCM != nil
}

func (r Router) Send(ctx context.Context, delivery Delivery) error {
	if delivery.PushToken == "" {
		return PermanentError{Err: errors.New("device push token is empty")}
	}

	switch delivery.Platform {
	case "ios":
		if r.APNs == nil {
			return ErrProviderUnavailable
		}
		return r.APNs.Send(ctx, delivery)
	case "android":
		if r.FCM == nil {
			return ErrProviderUnavailable
		}
		return r.FCM.Send(ctx, delivery)
	default:
		return PermanentError{Err: fmt.Errorf("unsupported push platform %q", delivery.Platform)}
	}
}

type eventPayload struct {
	TransferID  string `json:"transferId"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
}

func decodeEventPayload(raw json.RawMessage) eventPayload {
	var payload eventPayload
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func pushBody(payload eventPayload) string {
	if name := strings.TrimSpace(payload.DisplayName); name != "" {
		return name + " is ready on this device."
	}
	switch payload.Kind {
	case "photo":
		return "A photo is ready on this device."
	case "file":
		return "A file is ready on this device."
	case "link":
		return "A link is ready on this device."
	case "clipboard":
		return "Clipboard content is ready on this device."
	default:
		return "Something is ready on this device."
	}
}

type APNSConfig struct {
	KeyID         string
	TeamID        string
	Topic         string
	PrivateKeyPEM string
	Sandbox       bool
}

type APNSProvider struct {
	keyID   string
	teamID  string
	topic   string
	key     *ecdsa.PrivateKey
	baseURL string
	client  *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewAPNSProvider(cfg APNSConfig) (*APNSProvider, error) {
	block, _ := pem.Decode([]byte(cfg.PrivateKeyPEM))
	if block == nil {
		return nil, errors.New("decode APNs private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("APNs private key is not ECDSA")
	}
	if cfg.KeyID == "" || cfg.TeamID == "" || cfg.Topic == "" {
		return nil, errors.New("APNs key ID, team ID and topic are required")
	}

	baseURL := "https://api.push.apple.com"
	if cfg.Sandbox {
		baseURL = "https://api.sandbox.push.apple.com"
	}

	return &APNSProvider{
		keyID:   cfg.KeyID,
		teamID:  cfg.TeamID,
		topic:   cfg.Topic,
		key:     key,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *APNSProvider) Send(ctx context.Context, delivery Delivery) error {
	token, err := p.authorizationToken()
	if err != nil {
		return err
	}
	payload := decodeEventPayload(delivery.Payload)

	body, err := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": "PIXEL GO",
				"body":  pushBody(payload),
			},
			"sound":             "default",
			"content-available": 1,
		},
		"type":       delivery.EventType,
		"transferId": delivery.TransferID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/3/device/"+url.PathEscape(delivery.PushToken),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apns-topic", p.topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-collapse-id", delivery.TransferID)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	err = fmt.Errorf("APNs HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	if resp.StatusCode == 400 || resp.StatusCode == 403 || resp.StatusCode == 410 {
		return PermanentError{Err: err}
	}
	return err
}

func (p *APNSProvider) authorizationToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.cachedToken != "" && now.Before(p.tokenExpiry) {
		return p.cachedToken, nil
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": p.teamID,
		"iat": now.Unix(),
	})
	token.Header["kid"] = p.keyID

	signed, err := token.SignedString(p.key)
	if err != nil {
		return "", fmt.Errorf("sign APNs token: %w", err)
	}

	p.cachedToken = signed
	p.tokenExpiry = now.Add(50 * time.Minute)
	return signed, nil
}

type fcmServiceAccount struct {
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

type FCMProvider struct {
	projectID   string
	clientEmail string
	privateKey  *rsa.PrivateKey
	tokenURI    string
	baseURL     string
	client      *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func NewFCMProvider(serviceAccountJSON []byte) (*FCMProvider, error) {
	var cfg fcmServiceAccount
	if err := json.Unmarshal(serviceAccountJSON, &cfg); err != nil {
		return nil, fmt.Errorf("decode FCM service account: %w", err)
	}
	if cfg.ProjectID == "" || cfg.ClientEmail == "" || cfg.PrivateKey == "" {
		return nil, errors.New("FCM service account is incomplete")
	}
	if cfg.TokenURI == "" {
		cfg.TokenURI = "https://oauth2.googleapis.com/token"
	}

	block, _ := pem.Decode([]byte(cfg.PrivateKey))
	if block == nil {
		return nil, errors.New("decode FCM private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse FCM private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("FCM private key is not RSA")
	}

	return &FCMProvider{
		projectID:   cfg.ProjectID,
		clientEmail: cfg.ClientEmail,
		privateKey:  key,
		tokenURI:    cfg.TokenURI,
		baseURL:     "https://fcm.googleapis.com",
		client:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *FCMProvider) Send(ctx context.Context, delivery Delivery) error {
	token, err := p.oauthToken(ctx)
	if err != nil {
		return err
	}
	payload := decodeEventPayload(delivery.Payload)

	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": delivery.PushToken,
			"notification": map[string]string{
				"title": "PIXEL GO",
				"body":  pushBody(payload),
			},
			"data": map[string]string{
				"type":       delivery.EventType,
				"transferId": delivery.TransferID,
			},
			"android": map[string]any{
				"priority":     "HIGH",
				"collapse_key": delivery.TransferID,
			},
		},
	})
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(p.baseURL, "/") + "/v1/projects/" +
		url.PathEscape(p.projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	err = fmt.Errorf("FCM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	if resp.StatusCode == 400 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		return PermanentError{Err: err}
	}
	return err
}

func (p *FCMProvider) oauthToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.accessToken != "" && now.Add(time.Minute).Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	assertion := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   p.clientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   p.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	signed, err := assertion.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign FCM OAuth assertion: %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", signed)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.tokenURI,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf(
			"FCM OAuth HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(raw)),
		)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode FCM OAuth token: %w", err)
	}
	if result.AccessToken == "" {
		return "", errors.New("FCM OAuth token response missing access_token")
	}
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 3600
	}

	p.accessToken = result.AccessToken
	p.tokenExpiry = now.Add(time.Duration(result.ExpiresIn) * time.Second)
	return p.accessToken, nil
}
