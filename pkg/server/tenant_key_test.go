package server

import (
	"context"
	"crypto/sha256"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

const tenantBody = `{"account_url":"acc://a.acme","proof_class":"on_demand"}`

// fakeKeys serves API keys by the hash of their secret, as the api_keys table does.
type fakeKeys struct {
	mu   sync.Mutex
	keys map[[32]byte]*database.APIKey
}

func newFakeKeys() *fakeKeys { return &fakeKeys{keys: map[[32]byte]*database.APIKey{}} }

func (f *fakeKeys) add(secret string, k database.APIKey) *database.APIKey {
	f.mu.Lock()
	defer f.mu.Unlock()
	k.KeyID = uuid.New()
	f.keys[sha256.Sum256([]byte(secret))] = &k
	return &k
}

func (f *fakeKeys) GetAPIKeyByHash(_ context.Context, h []byte) (*database.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var a [32]byte
	copy(a[:], h)
	return f.keys[a], nil
}

func (f *fakeKeys) UpdateAPIKeyLastUsed(context.Context, uuid.UUID) error { return nil }

// fakeStore records the request the handler would insert.
type fakeStore struct {
	mu      sync.Mutex
	created []*database.NewBundleProofRequest
}

func (s *fakeStore) GetProofByTxHash(context.Context, string) (*database.ProofArtifact, error) {
	return nil, nil
}

func (s *fakeStore) CreateProofRequest(_ context.Context, in *database.NewBundleProofRequest) (*database.BundleProofRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created = append(s.created, in)
	return &database.BundleProofRequest{RequestID: uuid.New()}, nil
}

func tenantHandlers(keys *fakeKeys, store *fakeStore) *BundleHandlers {
	return &BundleHandlers{
		logger:          log.New(io.Discard, "", 0),
		rateLimiter:     NewRateLimiter(100),
		apiKeyValidator: NewAPIKeyValidatorWith(keys),
		requests:        store,
	}
}

func postRequest(h *BundleHandlers, target string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(tenantBody))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.HandleRequestProof(rr, req)
	return rr
}

func wantCode(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status || !strings.Contains(rr.Body.String(), code) {
		t.Fatalf("status %d body %s, want %d %s", rr.Code, rr.Body.String(), status, code)
	}
}

// A caller past the service token or the login with no API key is refused by name and nothing is stored.
// (On the old code the handler treated a missing key as an anonymous caller and went on to the store.)
func TestARequestWithoutAnAPIKeyIsRefusedByName(t *testing.T) {
	wantCode(t, requestProofWith(t, tenantBody), http.StatusUnauthorized, "API_KEY_REQUIRED")

	store := &fakeStore{}
	rr := postRequest(tenantHandlers(newFakeKeys(), store), "/api/v1/proofs/request", nil)
	wantCode(t, rr, http.StatusUnauthorized, "API_KEY_REQUIRED")
	if len(store.created) != 0 {
		t.Fatalf("%d requests were stored for a caller with no key", len(store.created))
	}
}

// A key in the URL is refused on a state-changing route: URLs are logged.
func TestAKeyInTheQueryStringIsNotAcceptedOnAMutatingRoute(t *testing.T) {
	keys := newFakeKeys()
	keys.add("s3cret", database.APIKey{ClientName: "org-a", CanRequestProofs: true, IsActive: true, RateLimitPerMin: 50})
	store := &fakeStore{}
	rr := postRequest(tenantHandlers(keys, store), "/api/v1/proofs/request?api_key=s3cret", nil)
	wantCode(t, rr, http.StatusUnauthorized, "API_KEY_REQUIRED")
	if len(store.created) != 0 {
		t.Fatal("a request was stored from a key in the URL")
	}
}

func TestAnUnknownOrInactiveOrExpiredKeyIsRefusedByName(t *testing.T) {
	keys := newFakeKeys()
	past := time.Now().Add(-time.Hour)
	keys.add("off", database.APIKey{ClientName: "org-off", CanRequestProofs: true, IsActive: false, RateLimitPerMin: 50})
	keys.add("old", database.APIKey{ClientName: "org-old", CanRequestProofs: true, IsActive: true, ExpiresAt: &past, RateLimitPerMin: 50})
	h := tenantHandlers(keys, &fakeStore{})
	for _, k := range []string{"nope", "off", "old"} {
		wantCode(t, postRequest(h, "/api/v1/proofs/request", map[string]string{"X-API-Key": k}), http.StatusUnauthorized, "INVALID_API_KEY")
	}
}

func TestAKeyWithoutTheRequestPermissionIsForbidden(t *testing.T) {
	keys := newFakeKeys()
	keys.add("ro", database.APIKey{ClientName: "org-ro", CanReadProofs: true, CanRequestProofs: false, IsActive: true, RateLimitPerMin: 50})
	store := &fakeStore{}
	wantCode(t, postRequest(tenantHandlers(keys, store), "/api/v1/proofs/request", map[string]string{"X-API-Key": "ro"}), http.StatusForbidden, "FORBIDDEN")
	if len(store.created) != 0 {
		t.Fatal("a request was stored for a key without the permission")
	}
}

// The request is stored against the key that made it.
func TestTheRequestIsStoredWithTheKeysID(t *testing.T) {
	keys := newFakeKeys()
	k := keys.add("good", database.APIKey{ClientName: "org-a", CanRequestProofs: true, IsActive: true, RateLimitPerMin: 50})
	store := &fakeStore{}
	rr := postRequest(tenantHandlers(keys, store), "/api/v1/proofs/request", map[string]string{"X-API-Key": "good"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s, want 202", rr.Code, rr.Body.String())
	}
	if len(store.created) != 1 || store.created[0].APIKeyID == nil || *store.created[0].APIKeyID != k.KeyID {
		t.Fatalf("stored request %+v, want api_key_id %s", store.created, k.KeyID)
	}
}

// Each key has its own limit, taken from the key's rate_limit_per_min, and one tenant exhausting it does not touch
// another, even one under the same client name.
func TestTheRateLimitIsPerKeyAndTakenFromTheKey(t *testing.T) {
	keys := newFakeKeys()
	keys.add("small", database.APIKey{ClientName: "same-name", CanRequestProofs: true, IsActive: true, RateLimitPerMin: 2})
	keys.add("big", database.APIKey{ClientName: "same-name", CanRequestProofs: true, IsActive: true, RateLimitPerMin: 100})
	h := tenantHandlers(keys, &fakeStore{})
	small := map[string]string{"X-API-Key": "small"}
	for i := 0; i < 2; i++ {
		if rr := postRequest(h, "/api/v1/proofs/request", small); rr.Code != http.StatusAccepted {
			t.Fatalf("request %d: %d %s", i, rr.Code, rr.Body.String())
		}
	}
	wantCode(t, postRequest(h, "/api/v1/proofs/request", small), http.StatusTooManyRequests, "RATE_LIMITED")
	if rr := postRequest(h, "/api/v1/proofs/request", map[string]string{"X-API-Key": "big"}); rr.Code != http.StatusAccepted {
		t.Fatalf("the other tenant was limited: %d %s", rr.Code, rr.Body.String())
	}
}

// Bulk export already demanded a key; its limit now comes from the key too.
func TestBulkExportRefusesAMissingKeyAndLimitsPerKey(t *testing.T) {
	keys := newFakeKeys()
	k := keys.add("bulk", database.APIKey{ClientName: "org-b", CanBulkDownload: true, IsActive: true, RateLimitPerMin: 1})
	h := &BulkHandlers{logger: log.New(io.Discard, "", 0), rateLimiter: NewRateLimiter(100), apiKeyValidator: NewAPIKeyValidatorWith(keys)}
	rr := httptest.NewRecorder()
	h.HandleBulkExport(rr, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/bulk/export", strings.NewReader(`{}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d %s", rr.Code, rr.Body.String())
	}
	if !h.rateLimiter.AllowWithLimit(k.KeyID.String(), k.RateLimitPerMin) {
		t.Fatal("the first call with a limit of 1 must pass")
	}
	if h.rateLimiter.AllowWithLimit(k.KeyID.String(), k.RateLimitPerMin) {
		t.Fatal("the second call with a limit of 1 must be refused")
	}
}
