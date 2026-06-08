package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// signTestToken builds an RS256 JWT (header.payload.sig) signed with priv.
func signTestToken(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()
	hdr := map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}
	hb, _ := json.Marshal(hdr)
	pb, _ := json.Marshal(claims)
	seg := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	h := sha256.Sum256([]byte(seg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return seg + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validClaims(project string) map[string]interface{} {
	now := time.Now().Unix()
	return map[string]interface{}{
		"aud": project,
		"iss": "https://securetoken.google.com/" + project,
		"sub": "user-abc-123",
		"exp": now + 3600,
		"iat": now - 10,
	}
}

func TestVerifyFirebaseIDToken(t *testing.T) {
	const project = "certen-web"
	const kid = "test-kid-1"
	t.Setenv("FIREBASE_PROJECT_ID", project)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Seed the cert cache so verification doesn't hit the network.
	fbCerts.mu.Lock()
	fbCerts.keys = map[string]*rsa.PublicKey{kid: &priv.PublicKey}
	fbCerts.expiresAt = time.Now().Add(time.Hour)
	fbCerts.mu.Unlock()

	t.Run("valid token", func(t *testing.T) {
		tok := signTestToken(t, priv, kid, validClaims(project))
		uid, reason := verifyFirebaseIDToken(tok)
		if uid != "user-abc-123" || reason != "" {
			t.Fatalf("expected success, got uid=%q reason=%q", uid, reason)
		}
	})

	t.Run("wrong aud", func(t *testing.T) {
		c := validClaims(project)
		c["aud"] = "some-other-project"
		uid, reason := verifyFirebaseIDToken(signTestToken(t, priv, kid, c))
		if uid != "" || reason != "aud-mismatch" {
			t.Fatalf("expected aud-mismatch, got uid=%q reason=%q", uid, reason)
		}
	})

	t.Run("wrong iss", func(t *testing.T) {
		c := validClaims(project)
		c["iss"] = "https://evil.example.com/" + project
		uid, reason := verifyFirebaseIDToken(signTestToken(t, priv, kid, c))
		if uid != "" || reason != "iss-mismatch" {
			t.Fatalf("expected iss-mismatch, got uid=%q reason=%q", uid, reason)
		}
	})

	t.Run("expired", func(t *testing.T) {
		c := validClaims(project)
		c["exp"] = time.Now().Unix() - 3600
		uid, reason := verifyFirebaseIDToken(signTestToken(t, priv, kid, c))
		if uid != "" || reason != "expired" {
			t.Fatalf("expected expired, got uid=%q reason=%q", uid, reason)
		}
	})

	t.Run("tampered signature", func(t *testing.T) {
		tok := signTestToken(t, priv, kid, validClaims(project))
		parts := strings.Split(tok, ".")
		// Decode the signature, flip a byte, re-encode — guarantees a changed
		// signature (flipping a trailing base64 char can be a no-op due to
		// padding bits).
		sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		sigBytes[0] ^= 0xFF
		tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sigBytes)
		uid, reason := verifyFirebaseIDToken(tampered)
		if uid != "" || reason != "signature-invalid" {
			t.Fatalf("expected signature-invalid, got uid=%q reason=%q", uid, reason)
		}
	})

	t.Run("wrong signing key", func(t *testing.T) {
		other, _ := rsa.GenerateKey(rand.Reader, 2048)
		tok := signTestToken(t, other, kid, validClaims(project))
		uid, reason := verifyFirebaseIDToken(tok)
		if uid != "" || reason != "signature-invalid" {
			t.Fatalf("expected signature-invalid, got uid=%q reason=%q", uid, reason)
		}
	})
}
