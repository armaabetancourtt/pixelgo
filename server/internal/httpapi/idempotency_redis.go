package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisIdempotencyLockTTL = 60 * time.Second

var errIdempotencyConflict = errors.New("idempotency key reused with a different request")

type redisIdempotencyRecord struct {
	Fingerprint string      `json:"fingerprint"`
	Status      int         `json:"status"`
	Header      http.Header `json:"header"`
	Body        []byte      `json:"body"`
}

type redisIdempotencyLock struct {
	Fingerprint string `json:"fingerprint"`
	Owner       string `json:"owner"`
}

func withRedisIdempotency(next http.Handler, client *redis.Client, ttl time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" || r.Method != http.MethodPost || strings.HasPrefix(r.URL.Path, "/v1/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		if len(key) > 128 {
			writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "idempotency key exceeds 128 characters")
			return
		}

		body, ok := readIdempotentBody(w, r)
		if !ok {
			return
		}
		fingerprint := requestFingerprint(r, body)
		storageKey := scopedIdempotencyKey(r, key)

		record, replayed, conflict, leader, err := acquireRedisIdempotency(
			r.Context(),
			client,
			storageKey,
			fingerprint,
			ttl,
		)
		if err != nil {
			// Idempotency coordination is a safety boundary for mutations.
			// If Redis is configured and unavailable, fail closed rather than
			// risk duplicating the operation across replicas.
			writeError(w, http.StatusServiceUnavailable, "idempotency_unavailable", "retry coordination is temporarily unavailable")
			return
		}
		if conflict {
			writeError(w, http.StatusConflict, "idempotency_key_reused", errIdempotencyConflict.Error())
			return
		}
		if replayed {
			writeStoredResponse(w, record, true)
			return
		}

		recorder := newBufferedResponseWriter()
		next.ServeHTTP(recorder, r)

		result := redisIdempotencyRecord{
			Fingerprint: fingerprint,
			Status:      recorder.status,
			Header:      recorder.header.Clone(),
			Body:        append([]byte(nil), recorder.body.Bytes()...),
		}

		if recorder.status >= http.StatusInternalServerError {
			_ = releaseRedisIdempotency(r.Context(), client, storageKey, leader)
		} else {
			_ = commitRedisIdempotency(r.Context(), client, storageKey, leader, result, ttl)
		}

		writeStoredResponse(w, result, false)
	})
}

func readIdempotentBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := readRequestBody(r)
	if err != nil {
		switch {
		case errors.Is(err, errRequestBodyTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB")
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
		}
		return nil, false
	}
	return body, true
}

func acquireRedisIdempotency(
	ctx context.Context,
	client *redis.Client,
	key string,
	fingerprint string,
	ttl time.Duration,
) (redisIdempotencyRecord, bool, bool, string, error) {
	resultKey, lockKey := redisIdempotencyKeys(key)

	for {
		record, found, err := getRedisIdempotencyResult(ctx, client, resultKey)
		if err != nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}
		if found {
			if record.Fingerprint != fingerprint {
				return redisIdempotencyRecord{}, false, true, "", nil
			}
			return record, true, false, "", nil
		}

		owner, err := randomOwner()
		if err != nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}
		lock := redisIdempotencyLock{
			Fingerprint: fingerprint,
			Owner:       owner,
		}
		lockData, err := json.Marshal(lock)
		if err != nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}

		acquired, err := client.SetNX(ctx, lockKey, lockData, redisIdempotencyLockTTL).Result()
		if err != nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}
		if acquired {
			return redisIdempotencyRecord{}, false, false, string(lockData), nil
		}

		existingLock, err := client.Get(ctx, lockKey).Bytes()
		if err != nil && err != redis.Nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}
		if err == redis.Nil {
			continue
		}

		var parsed redisIdempotencyLock
		if err := json.Unmarshal(existingLock, &parsed); err != nil {
			return redisIdempotencyRecord{}, false, false, "", err
		}
		if parsed.Fingerprint != fingerprint {
			return redisIdempotencyRecord{}, false, true, "", nil
		}

		select {
		case <-ctx.Done():
			return redisIdempotencyRecord{}, false, false, "", ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func getRedisIdempotencyResult(
	ctx context.Context,
	client *redis.Client,
	resultKey string,
) (redisIdempotencyRecord, bool, error) {
	data, err := client.Get(ctx, resultKey).Bytes()
	if err == redis.Nil {
		return redisIdempotencyRecord{}, false, nil
	}
	if err != nil {
		return redisIdempotencyRecord{}, false, err
	}

	var record redisIdempotencyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return redisIdempotencyRecord{}, false, err
	}
	return record, true, nil
}

var commitIdempotencyScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[3])
redis.call("DEL", KEYS[1])
return 1
`)

func commitRedisIdempotency(
	ctx context.Context,
	client *redis.Client,
	key string,
	owner string,
	record redisIdempotencyRecord,
	ttl time.Duration,
) error {
	resultKey, lockKey := redisIdempotencyKeys(key)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	_, err = commitIdempotencyScript.Run(
		ctx,
		client,
		[]string{lockKey, resultKey},
		owner,
		data,
		ttl.Milliseconds(),
	).Result()
	return err
}

var releaseIdempotencyScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func releaseRedisIdempotency(
	ctx context.Context,
	client *redis.Client,
	key string,
	owner string,
) error {
	_, lockKey := redisIdempotencyKeys(key)
	_, err := releaseIdempotencyScript.Run(
		ctx,
		client,
		[]string{lockKey},
		owner,
	).Result()
	return err
}

func redisIdempotencyKeys(key string) (string, string) {
	sum := requestKeyHash(key)
	base := "pixelgo:idempotency:" + sum
	return base + ":result", base + ":lock"
}

func requestKeyHash(key string) string {
	// Reuse SHA-256 so arbitrary caller-provided keys never become raw Redis keys.
	sum := sha256Bytes([]byte(key))
	return hex.EncodeToString(sum)
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func randomOwner() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func writeStoredResponse(w http.ResponseWriter, record redisIdempotencyRecord, replayed bool) {
	copyHeader(w.Header(), record.Header)
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.WriteHeader(record.Status)
	_, _ = w.Write(record.Body)
}
