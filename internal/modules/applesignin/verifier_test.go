package applesignin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testAud = "com.tasksy.app"

type fixture struct {
	key     *rsa.PrivateKey
	kid     string
	srv     *httptest.Server
	fetches atomic.Int32
	v       *Verifier
	now     time.Time
}

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func jwks(kid string, pub *rsa.PublicKey) map[string]any {
	return map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{key: newKey(t), kid: "TESTKID1", now: time.Now()}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.fetches.Add(1)
		json.NewEncoder(w).Encode(jwks(f.kid, &f.key.PublicKey))
	}))
	t.Cleanup(f.srv.Close)
	f.v = New([]string{testAud, "com.tasksy.web"}, f.srv.URL, f.srv.Client())
	f.v.now = func() time.Time { return f.now }
	return f
}

func (f *fixture) token(t *testing.T, mutate func(jwt.MapClaims), key *rsa.PrivateKey, kid string, method jwt.SigningMethod) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": Issuer, "aud": testAud, "sub": "001234.abcdef.5678",
		"iat": f.now.Add(-time.Minute).Unix(), "exp": f.now.Add(time.Hour).Unix(),
		"email": "jane@example.com", "email_verified": "true", "is_private_email": "false",
	}
	if mutate != nil {
		mutate(claims)
	}
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fixture) good(t *testing.T, mutate func(jwt.MapClaims)) string {
	return f.token(t, mutate, f.key, f.kid, jwt.SigningMethodRS256)
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	f := newFixture(t)
	id, err := f.v.Verify(context.Background(), f.good(t, nil), "")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "001234.abcdef.5678" || id.Email != "jane@example.com" || !id.EmailVerified || id.IsPrivateEmail {
		t.Fatalf("identity %+v", id)
	}
}

func TestVerifyBooleanClaims(t *testing.T) {
	f := newFixture(t)
	tok := f.good(t, func(c jwt.MapClaims) {
		c["email_verified"], c["is_private_email"] = true, true
		c["email"] = "abc@privaterelay.appleid.com"
	})
	id, err := f.v.Verify(context.Background(), tok, "")
	if err != nil || !id.EmailVerified || !id.IsPrivateEmail {
		t.Fatalf("%+v %v", id, err)
	}
}

func TestVerifyAcceptsSecondAudience(t *testing.T) {
	f := newFixture(t)
	if _, err := f.v.Verify(context.Background(), f.good(t, func(c jwt.MapClaims) { c["aud"] = "com.tasksy.web" }), ""); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejects(t *testing.T) {
	f := newFixture(t)
	other := newKey(t)
	cases := map[string]string{
		"wrong audience":     f.good(t, func(c jwt.MapClaims) { c["aud"] = "com.someone.else" }),
		"wrong issuer":       f.good(t, func(c jwt.MapClaims) { c["iss"] = "https://accounts.google.com" }),
		"expired":            f.good(t, func(c jwt.MapClaims) { c["exp"] = f.now.Add(-time.Hour).Unix() }),
		"no expiry":          f.good(t, func(c jwt.MapClaims) { delete(c, "exp") }),
		"issued in future":   f.good(t, func(c jwt.MapClaims) { c["iat"] = f.now.Add(time.Hour).Unix() }),
		"no subject":         f.good(t, func(c jwt.MapClaims) { delete(c, "sub") }),
		"empty subject":      f.good(t, func(c jwt.MapClaims) { c["sub"] = "" }),
		"signed by attacker": f.token(t, nil, other, f.kid, jwt.SigningMethodRS256),
		"unknown key id":     f.token(t, nil, f.key, "NOPE", jwt.SigningMethodRS256),
		"wrong algorithm":    f.token(t, nil, f.key, f.kid, jwt.SigningMethodRS384),
		"garbage":            "not.a.jwt",
		"empty":              "",
	}
	// alg=none and HS256 signed with the public key are the classic bypasses.
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"iss": Issuer, "aud": testAud, "sub": "x", "exp": f.now.Add(time.Hour).Unix()})
	none.Header["kid"] = f.kid
	cases["alg none"], _ = none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": Issuer, "aud": testAud, "sub": "x", "exp": f.now.Add(time.Hour).Unix()})
	hs.Header["kid"] = f.kid
	cases["HS256 with public key"], _ = hs.SignedString(f.key.N.Bytes())

	for name, tok := range cases {
		if _, err := f.v.Verify(context.Background(), tok, ""); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: want ErrInvalidToken, got %v", name, err)
		}
	}
}

func TestVerifyToleratesSmallClockSkew(t *testing.T) {
	f := newFixture(t)
	if _, err := f.v.Verify(context.Background(), f.good(t, func(c jwt.MapClaims) { c["exp"] = f.now.Add(-20 * time.Second).Unix() }), ""); err != nil {
		t.Fatalf("20s past expiry should pass within leeway: %v", err)
	}
}

func TestNonce(t *testing.T) {
	f := newFixture(t)
	raw := "raw-nonce-123"
	sum := sha256.Sum256([]byte(raw))
	hashed := hex.EncodeToString(sum[:])

	withHash := f.good(t, func(c jwt.MapClaims) { c["nonce"] = hashed })
	withRaw := f.good(t, func(c jwt.MapClaims) { c["nonce"] = raw })
	noNonce := f.good(t, nil)

	for _, c := range []struct {
		name, token, sent string
		ok                bool
	}{
		{"raw sent, token holds its hash", withHash, raw, true},
		{"hash sent, token holds the hash", withHash, hashed, true},
		{"raw sent, token holds the raw value", withRaw, raw, true},
		{"wrong nonce", withHash, "other", false},
		{"nonce sent but token has none", noNonce, raw, false},
		{"no nonce sent, token has one", withHash, "", true},
		{"no nonce either side", noNonce, "", true},
	} {
		_, err := f.v.Verify(context.Background(), c.token, c.sent)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestNotConfiguredRejectsEverything(t *testing.T) {
	f := newFixture(t)
	v := New(nil, f.srv.URL, f.srv.Client())
	v.now = f.v.now
	if _, err := v.Verify(context.Background(), f.good(t, nil), ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestKeysAreCachedAndRefetchIsRateLimited(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		if _, err := f.v.Verify(context.Background(), f.good(t, nil), ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.fetches.Load(); n != 1 {
		t.Fatalf("fetched keys %d times, want 1", n)
	}
	// Tokens with unknown key ids must not make us hit Apple every time.
	bad := f.token(t, nil, f.key, "UNKNOWN", jwt.SigningMethodRS256)
	for i := 0; i < 5; i++ {
		f.v.Verify(context.Background(), bad, "")
	}
	if n := f.fetches.Load(); n != 1 {
		t.Fatalf("unknown kid caused %d fetches within a minute, want 1", n)
	}
	// After the gap a rotated key is picked up.
	f.key, f.kid = newKey(t), "ROTATED"
	f.now = f.now.Add(2 * time.Minute)
	if _, err := f.v.Verify(context.Background(), f.good(t, nil), ""); err != nil {
		t.Fatalf("rotated key not picked up: %v", err)
	}
}

func TestKeyFetchFailure(t *testing.T) {
	f := newFixture(t)
	tok := f.good(t, nil)
	f.srv.Close()
	_, err := f.v.Verify(context.Background(), tok, "")
	if err == nil || errors.Is(err, ErrInvalidToken) {
		t.Fatalf("an unreachable Apple must not look like a bad token (that would sign users out): %v", err)
	}
}

func TestStaleKeyUsedWhenAppleIsDown(t *testing.T) {
	f := newFixture(t)
	tok := f.good(t, nil)
	if _, err := f.v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatal(err)
	}
	f.srv.Close()
	f.now = f.now.Add(3 * time.Hour)
	tok = f.good(t, nil)
	if _, err := f.v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("stale cached key should still verify: %v", err)
	}
}
