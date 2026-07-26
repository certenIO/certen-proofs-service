package server

import (
	"context"
	"net/http"
	"path"
)

// Preserving the request path exactly as the caller sent it.
//
// Accumulate transaction hashes are URLs: acc://<hash>@<adi>/data. The gateway
// percent-encodes one into a path segment, so the wire path is
//
//	/api/v1/proofs/tx/acc%3A%2F%2F<hash>%40<adi>%2Fdata
//
// which decodes to a path containing a DOUBLE SLASH. net/http's ServeMux
// compares the decoded path against path.Clean and, when they differ, answers
// with a 301 to the cleaned form — collapsing acc:// to acc:/ .
//
// That single redirect broke the whole proof pipeline in two ways:
//
//  1. The caller's service token is one-time (nonce-bound). Following the
//     redirect replays it, and the replay guard rejects the second request —
//     so an authenticated caller saw a permanent 401 with no way to recover.
//  2. Even if it authenticated, the handler would read the CLEANED path and
//     look up acc:/… , which matches no stored proof.
//
// The fix is to never let the redirect happen: clean the path ourselves before
// ServeMux sees it (so its comparison passes) while carrying the original path
// forward for handlers that need the true value. This changes no route and no
// public contract; it only removes a redirect that should never have been on
// this path.
type rawPathCtxKey struct{}

// WithRawPath stores the caller's original, uncleaned path.
func WithRawPath(ctx context.Context, p string) context.Context {
	return context.WithValue(ctx, rawPathCtxKey{}, p)
}

// RawPath returns the path exactly as the caller sent it (decoded but NOT
// cleaned), falling back to r.URL.Path when no rewrite occurred. Handlers that
// parse a resource identifier out of the path should use this rather than
// r.URL.Path, or an identifier containing "//" will silently lose a slash.
func RawPath(r *http.Request) string {
	if v, ok := r.Context().Value(rawPathCtxKey{}).(string); ok && v != "" {
		return v
	}
	return r.URL.Path
}

// cleanPathLikeServeMux reproduces net/http's unexported cleanPath, so we
// normalize to exactly the value ServeMux would have redirected to. Matching it
// exactly is what guarantees ServeMux finds nothing left to redirect.
func cleanPathLikeServeMux(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean drops a trailing slash; ServeMux keeps it, and route matching
	// for the "/api/v1/proofs/" style prefix patterns depends on it.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// PreserveRawPathMiddleware must sit immediately outside the mux (and INSIDE
// the auth middleware, which verifies the HMAC over the untouched wire path).
func PreserveRawPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orig := r.URL.Path
		cleaned := cleanPathLikeServeMux(orig)
		if cleaned == orig {
			next.ServeHTTP(w, r)
			return
		}
		// Clone rather than mutate: the original *http.Request may be observed
		// by outer middleware (logging, metrics) after this handler returns.
		r2 := r.Clone(WithRawPath(r.Context(), orig))
		r2.URL.Path = cleaned
		next.ServeHTTP(w, r2)
	})
}
