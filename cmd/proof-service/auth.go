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
// Rollout is gated by AUTH_REQUIRED:
//   - unset/false -> LOG-ONLY: verify + log, but never reject.
//   - "true"      -> ENFORCE: reject unauthenticated/invalid requests with 401.
//
// NOTE: this only accepts the service token. The web app calls the proof
// service directly with a Firebase ID token; until a Firebase verifier + the
// web-side interceptor for the proofs origin are added, DO NOT set
// AUTH_REQUIRED=true here or direct browser calls will be rejected.

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
)

const (
	authToleranceSec = 300
	authNonceTTLSec  = 300
)

var (
	authSeenNonces   = map[string]int64{}
	authSeenNoncesMu sync.Mutex
)

func authEnforce() bool { return os.Getenv("AUTH_REQUIRED") == "true" }

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

	authSeenNoncesMu.Lock()
	for k, exp := range authSeenNonces {
		if exp < now {
			delete(authSeenNonces, k)
		}
	}
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
			// bytes the caller signed, and handlers can still read it.
			var body []byte
			if r.Body != nil {
				if b, err := io.ReadAll(r.Body); err == nil {
					body = b
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}

			reason := "no-credentials"
			if header := r.Header.Get("X-Certen-Service-Token"); header != "" {
				res := authVerifyServiceToken(header, r.Method, r.URL.Path, r.URL.RawQuery, body)
				if res.ok {
					if !authEnforce() {
						logger.Printf("[auth:ok] service kv=%s %s %s", res.version, r.Method, r.URL.Path)
					}
					next.ServeHTTP(w, r)
					return
				}
				reason = "service-token:" + res.reason
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
