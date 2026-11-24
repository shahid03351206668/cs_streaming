// Package applesignin verifies "Sign in with Apple" identity tokens.
//
// The mobile app gets an identity token (a JWT signed by Apple) from the
// system sign-in sheet and posts it to the API. The token is only trusted after
// its RS256 signature is checked against Apple's published keys and its issuer,
// audience, expiry and (when supplied) nonce are verified.
package applesignin

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	Issuer      = "https://appleid.apple.com"
	DefaultKeys = "https://appleid.apple.com/auth/keys"

	keyTTL        = time.Hour
	refetchEvery  = time.Minute // minimum gap between key fetches, so a bad kid can't hammer Apple
	maxKeysBody   = 1 << 20
	maxClockSkew  = time.Minute
	maxTokenBytes = 8192
)

var (
	ErrNotConfigured = errors.New("apple sign-in is not configured")
	ErrInvalidToken  = errors.New("invalid apple identity token")
)

// Identity is what a verified token says about the user.
type Identity struct {
	Subject        string // stable, unique per user and app team
	Email          string
	EmailVerified  bool
	IsPrivateEmail bool
}

type Verifier struct {
	clientIDs []string // accepted audiences: app bundle ID and/or web Services ID
	keysURL   string
	client    *http.Client
	now       func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// NewFromEnv reads APPLE_CLIENT_IDS (comma separated): the iOS bundle ID and,
// for web or Android sign-in, the Services ID. With none set the verifier
// rejects everything rather than accept any audience. APPLE_KEYS_URL overrides
// Apple's key endpoint and exists for tests against a stub.
func NewFromEnv() *Verifier {
	var ids []string
	for _, id := range strings.Split(os.Getenv("APPLE_CLIENT_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	keysURL := os.Getenv("APPLE_KEYS_URL")
	if keysURL == "" {
		keysURL = DefaultKeys
	}
	return New(ids, keysURL, &http.Client{Timeout: 10 * time.Second})
}

func New(clientIDs []string, keysURL string, client *http.Client) *Verifier {
	return &Verifier{clientIDs: clientIDs, keysURL: keysURL, client: client, now: time.Now}
}

func (v *Verifier) Configured() bool { return len(v.clientIDs) > 0 }

// Verify checks the token. nonce is the value the app sent in its sign-in
// request, either raw or already SHA-256 hashed; pass "" if the app uses none.
func (v *Verifier) Verify(ctx context.Context, idToken, nonce string) (*Identity, error) {
	if !v.Configured() {
		return nil, ErrNotConfigured
	}
	if idToken == "" || len(idToken) > maxTokenBytes {
		return nil, ErrInvalidToken
	}

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no key id")
		}
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(maxClockSkew),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		if errors.Is(err, errKeysUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !v.audienceOK(claims) {
		return nil, fmt.Errorf("%w: wrong audience", ErrInvalidToken)
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("%w: no subject", ErrInvalidToken)
	}
	if nonce != "" && !nonceOK(claims, nonce) {
		return nil, fmt.Errorf("%w: nonce mismatch", ErrInvalidToken)
	}

	email, _ := claims["email"].(string)
	return &Identity{
		Subject:        sub,
		Email:          strings.TrimSpace(email),
		EmailVerified:  truthy(claims["email_verified"]),
		IsPrivateEmail: truthy(claims["is_private_email"]),
	}, nil
}

func (v *Verifier) audienceOK(claims jwt.MapClaims) bool {
	aud, err := claims.GetAudience()
	if err != nil {
		return false
	}
	for _, a := range aud {
		for _, id := range v.clientIDs {
			if subtle.ConstantTimeCompare([]byte(a), []byte(id)) == 1 {
				return true
			}
		}
	}
	return false
}

// nonceOK accepts the nonce as sent (already hashed by the app) or the raw
// value, whose SHA-256 hex digest is what Apple puts in the token.
func nonceOK(claims jwt.MapClaims, nonce string) bool {
	got, _ := claims["nonce"].(string)
	if got == "" {
		return false
	}
	sum := sha256.Sum256([]byte(nonce))
	for _, want := range []string{nonce, hex.EncodeToString(sum[:])} {
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			return true
		}
	}
	return false
}

// truthy handles Apple sending booleans as either true or "true".
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	}
	return false
}

var errKeysUnavailable = errors.New("could not fetch apple signing keys")

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	fresh := v.keys != nil && v.now().Sub(v.fetchedAt) < keyTTL
	if k, ok := v.keys[kid]; ok && fresh {
		return k, nil
	}
	// Unknown kid or stale cache: refresh, but not more than once a minute.
	if v.keys == nil || v.now().Sub(v.fetchedAt) >= refetchEvery {
		if err := v.fetch(ctx); err != nil {
			if k, ok := v.keys[kid]; ok { // serve a stale key rather than fail sign-in
				return k, nil
			}
			return nil, fmt.Errorf("%w: %v", errKeysUnavailable, err)
		}
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown signing key")
}

func (v *Verifier) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.keysURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		v.fetchedAt = v.now() // back off even on failure
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		v.fetchedAt = v.now()
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxKeysBody)).Decode(&set); err != nil {
		v.fetchedAt = v.now()
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(n) == 0 || len(e) == 0 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		v.fetchedAt = v.now()
		return errors.New("no usable keys in response")
	}
	v.keys, v.fetchedAt = keys, v.now()
	return nil
}
