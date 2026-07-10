// Copyright 2025 Certen Protocol
//
// Service-to-service auth middleware (dual-mode) for the proof service.
//
// Verifies the HMAC service token the gateway (and other internal callers)
// stamp in X-Certen-Service-Token. Byte-exact with api-gateway's
// src/clients/downstream-auth.ts (see api-gateway/docs/service-token-verify.md):
//
//	X-Certen-Service-Token: t=<unix>,m=<METHOD>,kv=<ver>,n=<nonce>,v1=<hex>
//	HMAC input: "${t}.${METHOD}.${canonicalPath}.${bodyLen}.${bodyHash}.${nonce}"
//
// It also accepts the web app's Firebase ID token in `Authorization: Bearer`
// (the web app calls the proof service directly) — see firebase_auth.go.
//
// Rollout is gated by AUTH_REQUIRED:
//   - unset/false -> LOG-ONLY: verify + log, but never reject.
//   - "true"      -> ENFORCE: reject unauthenticated/invalid requests with 401.
//
// Before flipping AUTH_REQUIRED=true: deploy this image (Go Firebase verifier),
// set FIREBASE_PROJECT_ID, and ensure the web app's fetch interceptor attaches
// the Firebase token to proof-service requests. Internal callers (gateway,
// api-bridge proxy) already sign with the service token.

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/certen/proofs-service/pkg/server"
)

const (
	authToleranceSec = 300
	authNonceTTLSec  = 300
)

var (
	authSeenNonces   = map[string]int64{}
	authSeenNoncesMu sync.Mutex
)

// authEnforce reports whether unauthenticated/invalid requests are rejected.
// Fail-closed by default (P2): if AUTH_REQUIRED is set we honor it; if it is
// unset we ENFORCE unless DEVELOPMENT_MODE=true. A forgotten env var must never
// silently serve the proof corpus unauthenticated.
func authEnforce() bool {
	if v := os.Getenv("AUTH_REQUIRED"); v != "" {
		return v == "true"
	}
	return os.Getenv("DEVELOPMENT_MODE") != "true"
}

// startAuthNonceReaper sweeps expired replay-protection nonces on a timer so
// the per-request hot path is O(1) (was an O(n) scan of the whole map under a
// global mutex on every authenticated request).
func startAuthNonceReaper() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now().Unix()
			authSeenNoncesMu.Lock()
			for k, exp := range authSeenNonces {
				if exp < now {
					delete(authSeenNonces, k)
				}
			}
			authSeenNoncesMu.Unlock()
		}
	}()
}

func authIsPublicPath(p string) bool {
	return p == "/health" || p == "/api/v1/system/health"
}

func authFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func authServiceSecrets() map[string]string {
	out := map[string]string{}
	if v := authFirstNonEmpty(os.Getenv("PROOFS_SERVICE_TOKEN_SECRET_V1"), os.Getenv("PROOFS_SERVICE_TOKEN_SECRET")); v != "" {
		out["v1"] = v
	}
	if v := os.Getenv("PROOFS_SERVICE_TOKEN_SECRET_V2"); v != "" {
		out["v2"] = v
	}
	return out
}

// authCanonicalPath mirrors downstream-auth.ts canonicalPath: it sorts the RAW
// query string by key (no decoding) so signer and verifier agree byte-for-byte.
func authCanonicalPath(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	type kv struct{ k, v string }
	pairs := strings.Split(rawQuery, "&")
	items := make([]kv, 0, len(pairs))
	for _, p := range pairs {
		if p == "" {
			continue
		}
		if eq := strings.Index(p, "="); eq < 0 {
			items = append(items, kv{p, ""})
		} else {
			items = append(items, kv{p[:eq], p[eq+1:]})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].k < items[j].k })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.k + "=" + it.v
	}
	return path + "?" + strings.Join(out, "&")
}

func authAbs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

type authResult struct {
	ok      bool
	reason  string
	version string
}

func authVerifyServiceToken(header, method, path, rawQuery string, body []byte) authResult {
	secrets := authServiceSecrets()
	if len(secrets) == 0 {
		return authResult{false, "not-configured", ""}
	}

	parts := map[string]string{}
	for _, seg := range strings.Split(header, ",") {
		eq := strings.Index(seg, "=")
		if eq < 0 {
			continue
		}
		parts[strings.TrimSpace(seg[:eq])] = strings.TrimSpace(seg[eq+1:])
	}
	tStr := parts["t"]
	m := parts["m"]
	kv := parts["kv"]
	nonce := parts["n"]
	sig := parts["v1"]
	if kv == "" {
		kv = "v1"
	}
	if tStr == "" || m == "" || nonce == "" || sig == "" {
		return authResult{false, "missing-fields", ""}
	}
	t, err := strconv.ParseInt(tStr, 10, 64)
	if err != nil {
		return authResult{false, "bad-timestamp", ""}
	}
	if !strings.EqualFold(m, method) {
		return authResult{false, "method-mismatch", ""}
	}
	now := time.Now().Unix()
	if authAbs64(now-t) > authToleranceSec {
		return authResult{false, "stale", ""}
	}

	// O(1) replay check; expiry is swept by startAuthNonceReaper, not here.
	authSeenNoncesMu.Lock()
	_, dup := authSeenNonces[nonce]
	authSeenNoncesMu.Unlock()
	if dup {
		return authResult{false, "replay", ""}
	}

	secret := secrets[kv]
	if secret == "" {
		return authResult{false, "unknown-kv-" + kv, ""}
	}

	bodyLen := len(body)
	bodyHash := ""
	if bodyLen > 0 {
		sum := sha256.Sum256(body)
		bodyHash = hex.EncodeToString(sum[:])
	}
	input := tStr + "." + m + "." + authCanonicalPath(path, rawQuery) + "." + strconv.Itoa(bodyLen) + "." + bodyHash + "." + nonce
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return authResult{false, "bad-signature", ""}
	}

	authSeenNoncesMu.Lock()
	authSeenNonces[nonce] = now + authNonceTTLSec
	authSeenNoncesMu.Unlock()
	return authResult{true, "", kv}
}

// authMiddleware wraps the handler chain. Place it OUTSIDE corsMiddleware so it
// can short-circuit before CORS while still letting preflight through.
func authMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || authIsPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Read + restore the body so the HMAC can be checked over the exact
			// bytes the caller signed, and handlers can still read it. Bound the
			// body (P4) so a giant POST can't be buffered into memory here.
			const maxBodyBytes = 4 << 20 // 4 MiB
			var body []byte
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
				if b, err := io.ReadAll(r.Body); err == nil {
					body = b
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}

			reason := "no-credentials"
			if header := r.Header.Get("X-Certen-Service-Token"); header != "" {
				// Verify over the RAW wire path (EscapedPath), not the percent-decoded
				// r.URL.Path. The gateway signs the encoded path (e.g. an acc:// tx hash
				// as acc%3A%2F%2F…); decoding here made the HMAC input diverge only for
				// paths with reserved chars, 401'ing every proof-by-tx-hash lookup while
				// simple UUID paths (encode==decode) worked. RawQuery is already raw.
				res := authVerifyServiceToken(header, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, body)
				if res.ok {
					if !authEnforce() {
						logger.Printf("[auth:ok] service kv=%s %s %s", res.version, r.Method, r.URL.Path)
					}
					ctx := server.WithPrincipal(r.Context(), server.Principal{Type: server.PrincipalService})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				reason = "service-token:" + res.reason
			} else if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				// Web app: Firebase ID token.
				token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
				uid, fbReason := verifyFirebaseIDToken(token)
				if uid != "" {
					if !authEnforce() {
						logger.Printf("[auth:ok] firebase uid=%s %s %s", uid, r.Method, r.URL.Path)
					}
					ctx := server.WithPrincipal(r.Context(), server.Principal{Type: server.PrincipalUser, UID: uid})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				reason = "firebase:" + fbReason
			}

			if authEnforce() {
				logger.Printf("[auth] 401 %s %s — %s", r.Method, r.URL.Path, reason)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"success":false,"error":"Unauthorized"}`))
				return
			}

			logger.Printf("[auth:log-only] %s %s — %s (allowed; set AUTH_REQUIRED=true to enforce)", r.Method, r.URL.Path, reason)
			next.ServeHTTP(w, r)
		})
	}
}

func logAuthStatus(logger *log.Logger) {
	mode := "log-only"
	if authEnforce() {
		mode = "ENFORCE"
	}
	logger.Printf("Auth middleware: mode=%s serviceToken=%v", mode, len(authServiceSecrets()) > 0)
}
