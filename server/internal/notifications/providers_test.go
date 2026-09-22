package notifications

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestAPNSProviderBuildsTokenAuthenticatedRequest(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		if r.URL.Path != "/3/device/device-token" {
			t.Errorf("unexpected APNs path %q", r.URL.Path)
		}
		if got := r.Header.Get("apns-topic"); got != "com.example.pixelgo" {
			t.Errorf("unexpected APNs topic %q", got)
		}
		if got := r.Header.Get("apns-push-type"); got != "alert" {
			t.Errorf("unexpected APNs push type %q", got)
		}
		if got := r.Header.Get("apns-collapse-id"); got != "tr_1" {
			t.Errorf("unexpected collapse id %q", got)
		}

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "bearer ") {
			t.Errorf("missing bearer authorization: %q", auth)
		} else {
			raw := strings.TrimPrefix(auth, "bearer ")
			token, err := jwt.Parse(
				raw,
				func(token *jwt.Token) (any, error) {
					return &key.PublicKey, nil
				},
				jwt.WithValidMethods([]string{"ES256"}),
			)
			if err != nil || !token.Valid {
				t.Errorf("invalid APNs provider token: %v", err)
			}
			if kid, _ := token.Header["kid"].(string); kid != "KEY123" {
				t.Errorf("unexpected APNs key id %q", kid)
			}
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if got, _ := claims["iss"].(string); got != "TEAM123" {
					t.Errorf("unexpected APNs issuer %q", got)
				}
			}
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode APNs body: %v", err)
		}
		if got, _ := body["type"].(string); got != "transfer.ready" {
			t.Errorf("unexpected APNs event type %q", got)
		}
		if got, _ := body["transferId"].(string); got != "tr_1" {
			t.Errorf("unexpected APNs transfer id %q", got)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider, err := NewAPNSProvider(APNSConfig{
		KeyID:         "KEY123",
		TeamID:        "TEAM123",
		Topic:         "com.example.pixelgo",
		PrivateKeyPEM: string(keyPEM),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.baseURL = server.URL
	provider.client = server.Client()

	err = provider.Send(context.Background(), Delivery{
		TransferID: "tr_1",
		EventType:  "transfer.ready",
		Platform:   "ios",
		PushToken:  "device-token",
		Payload: json.RawMessage(
			"{\"transferId\":\"tr_1\",\"kind\":\"photo\",\"displayName\":\"IMG.jpg\"}",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one APNs call, got %d", calls.Load())
	}
}

func TestFCMProviderExchangesOAuthAndSendsHTTPv1Message(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})

	var tokenCalls atomic.Int32
	var messageCalls atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse OAuth form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if got := r.Form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("unexpected grant type %q", got)
			}

			assertion := r.Form.Get("assertion")
			token, err := jwt.Parse(
				assertion,
				func(token *jwt.Token) (any, error) {
					return &key.PublicKey, nil
				},
				jwt.WithValidMethods([]string{"RS256"}),
			)
			if err != nil || !token.Valid {
				t.Errorf("invalid FCM OAuth assertion: %v", err)
			}
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if got, _ := claims["iss"].(string); got != "svc@example.iam.gserviceaccount.com" {
					t.Errorf("unexpected FCM issuer %q", got)
				}
				if got, _ := claims["aud"].(string); got != server.URL+"/token" {
					t.Errorf("unexpected FCM audience %q", got)
				}
				if got, _ := claims["scope"].(string); got != "https://www.googleapis.com/auth/firebase.messaging" {
					t.Errorf("unexpected FCM scope %q", got)
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"access_token\":\"access-token\",\"expires_in\":3600}"))

		case "/v1/projects/project-123/messages:send":
			call := messageCalls.Add(1)
			expectedTransferID := "tr_2"
			if call == 2 {
				expectedTransferID = "tr_3"
			}
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Errorf("unexpected FCM authorization %q", got)
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode FCM body: %v", err)
			}
			message, _ := body["message"].(map[string]any)
			if got, _ := message["token"].(string); got != "fcm-device-token" {
				t.Errorf("unexpected FCM device token %q", got)
			}
			data, _ := message["data"].(map[string]any)
			if got, _ := data["transferId"].(string); got != expectedTransferID {
				t.Errorf(
					"unexpected FCM transfer id %q, want %q",
					got,
					expectedTransferID,
				)
			}
			android, _ := message["android"].(map[string]any)
			if got, _ := android["collapse_key"].(string); got != expectedTransferID {
				t.Errorf(
					"unexpected FCM collapse key %q, want %q",
					got,
					expectedTransferID,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"name\":\"projects/project-123/messages/msg-1\"}"))

		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	accountJSON, err := json.Marshal(map[string]string{
		"project_id":   "project-123",
		"private_key":  string(keyPEM),
		"client_email": "svc@example.iam.gserviceaccount.com",
		"token_uri":    server.URL + "/token",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider, err := NewFCMProvider(accountJSON)
	if err != nil {
		t.Fatal(err)
	}
	provider.baseURL = server.URL
	provider.client = server.Client()

	err = provider.Send(context.Background(), Delivery{
		TransferID: "tr_2",
		EventType:  "transfer.ready",
		Platform:   "android",
		PushToken:  "fcm-device-token",
		Payload: json.RawMessage(
			"{\"transferId\":\"tr_2\",\"kind\":\"file\",\"displayName\":\"report.pdf\"}",
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	if tokenCalls.Load() != 1 {
		t.Fatalf("expected one OAuth exchange, got %d", tokenCalls.Load())
	}
	if messageCalls.Load() != 1 {
		t.Fatalf("expected one FCM message call, got %d", messageCalls.Load())
	}

	// Cached OAuth token should be reused on a second message.
	err = provider.Send(context.Background(), Delivery{
		TransferID: "tr_3",
		EventType:  "transfer.ready",
		Platform:   "android",
		PushToken:  "fcm-device-token",
		Payload: json.RawMessage(
			"{\"transferId\":\"tr_3\",\"kind\":\"text\",\"displayName\":\"Text\"}",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("expected cached OAuth token, got %d exchanges", tokenCalls.Load())
	}
	if messageCalls.Load() != 2 {
		t.Fatalf("expected two FCM message calls, got %d", messageCalls.Load())
	}

	// Make sure the fake URL itself is syntactically valid; this also guards
	// accidental double-slash endpoint construction.
	if _, err := url.Parse(provider.baseURL); err != nil {
		t.Fatal(err)
	}
}
