// Copyright 2025 Certen Protocol
//
// Authenticated-principal plumbing for per-tenant authorization.
//
// The auth middleware (cmd/proof-service/auth.go) verifies *who* a caller is;
// this carries that identity into handlers so they can enforce *what* the
// caller may access. Two principal kinds:
//   - service: the gateway / internal callers (HMAC service token). The gateway
//     enforces tenant isolation upstream, so a service principal is trusted to
//     read across users.
//   - user: the web app (Firebase ID token), scoped to its own uid.

package server

import "context"

type PrincipalType string

const (
	PrincipalService PrincipalType = "service"
	PrincipalUser    PrincipalType = "user"
)

// Principal is the authenticated caller identity attached to the request ctx.
type Principal struct {
	Type PrincipalType
	UID  string // set when Type == PrincipalUser
}

type principalCtxKey struct{}

// WithPrincipal returns a context carrying the authenticated principal.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom extracts the principal, if any, from the request context.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}

// canAccessUser reports whether the caller may read data scoped to userID.
// A service caller may access any user; a user caller only their own. When no
// principal is present the request is in log-only/dev mode (enforcement 401s
// before reaching a handler), so we allow it to preserve dev behavior.
func canAccessUser(ctx context.Context, userID string) bool {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return true
	}
	if p.Type == PrincipalService {
		return true
	}
	return p.UID == userID
}
