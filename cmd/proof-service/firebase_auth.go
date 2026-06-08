// Copyright 2025 Certen Protocol
//
// Firebase ID-token verifier (stdlib only) for the proof service.
//
// The web app calls the proof service directly with a Firebase ID token in
// `Authorization: Bearer <jwt>`. These are RS256 JWTs signed by Google. We
// verify them the same way firebase-admin does, without pulling in any SDK:
//
//   1. Fetch Google's securetoken x509 certs (PEM, keyed by `kid`), cached per
//      the response's Cache-Control max-age.
//   2. Verify the RS256 signature over `header.payload` with the matching cert.
//   3. Validate aud == project id, iss == securetoken issuer, exp/iat, and a
//      non-empty sub (the uid).
//
// Configured via FIREBASE_PROJECT_ID (falls back to GOOGLE_CLOUD_PROJECT).

package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const googleSecureTokenCertsURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

func firebaseProjectID() string {
	return authFirstNonEmpty(os.Getenv("FIREBASE_PROJECT_ID"), os.Getenv("GOOGLE_CLOUD_PROJECT"))
}

// firebaseCertCache holds Google's current securetoken signing certs.
type firebaseCertCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
}

var fbCerts = &firebaseCertCache{keys: map[string]*rsa.PublicKey{}}

func (c *firebaseCertCache) get(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	if time.Now().Before(c.expiresAt) {
		if k, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return k, nil
		}
	}
	c.mu.RUnlock()

	if err := c.refresh(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown key id %q", kid)
}

func (c *firebaseCertCache) refresh() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(googleSecureTokenCertsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("certs fetch status %d", resp.StatusCode)
	}

	var pems map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&pems); err != nil {
		return err
	}

	keys := map[string]*rsa.PublicKey{}
	for kid, pemStr := range pems {
		block, _ := pem.Decode([]byte(pemStr))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if pk, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			keys[kid] = pk
		}
	}

	// Honor Cache-Control max-age; default to 1h.
	ttl := time.Hour
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		for _, part := range strings.Split(cc, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "max-age=") {
				if secs, err := strconv.Atoi(strings.TrimPrefix(part, "max-age=")); err == nil && secs > 0 {
					ttl = time.Duration(secs) * time.Second
				}
			}
		}
	}

	c.mu.Lock()
	c.keys = keys
	c.expiresAt = time.Now().Add(ttl)
	c.mu.Unlock()
	return nil
}

type firebaseClaims struct {
	Aud string `json:"aud"`
	Iss string `json:"iss"`
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

// verifyFirebaseIDToken validates a Firebase ID token and returns (uid, reason).
// A non-empty uid with reason "" means success.
func verifyFirebaseIDToken(token string) (string, string) {
	projectID := firebaseProjectID()
	if projectID == "" {
		return "", "firebase-not-configured"
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "malformed-jwt"
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "bad-header"
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return "", "bad-header"
	}
	if hdr.Alg != "RS256" {
		return "", "bad-alg"
	}
	if hdr.Kid == "" {
		return "", "missing-kid"
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "bad-payload"
	}
	var claims firebaseClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return "", "bad-payload"
	}

	now := time.Now().Unix()
	const leeway = 60
	if claims.Exp == 0 || now > claims.Exp+leeway {
		return "", "expired"
	}
	if claims.Iat != 0 && claims.Iat > now+leeway {
		return "", "iat-in-future"
	}
	if claims.Aud != projectID {
		return "", "aud-mismatch"
	}
	if claims.Iss != "https://securetoken.google.com/"+projectID {
		return "", "iss-mismatch"
	}
	if claims.Sub == "" {
		return "", "missing-sub"
	}

	pubKey, err := fbCerts.get(hdr.Kid)
	if err != nil {
		return "", "unknown-kid"
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "bad-signature-encoding"
	}
	signed := parts[0] + "." + parts[1]
	hashed := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], sig); err != nil {
		return "", "signature-invalid"
	}

	return claims.Sub, ""
}
