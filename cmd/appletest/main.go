// appletest checks Sign in with Apple end to end against a local server. It
// plays Apple: it serves a signing key at -keys-addr and mints identity tokens,
// so the server must be started with
//
//	APPLE_CLIENT_IDS=com.tasksy.test APPLE_KEYS_URL=http://127.0.0.1:8099/keys
//
// Needs the server's local DB (-db) to check disabled and admin accounts.
//
//	go run ./cmd/appletest -db "$DB_URI"
//
// The tool makes a new signing key each run and the server caches keys, so
// restart the server between runs.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	aud = "com.tasksy.test"
	kid = "APPLETEST1"
)

type suite struct {
	api *API
	r   *Runner
	g   *gorm.DB
	key *rsa.PrivateKey
	run int64
}

func (s *suite) token(mutate func(jwt.MapClaims)) string {
	claims := jwt.MapClaims{
		"iss": "https://appleid.apple.com", "aud": aud,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		"email_verified": "true", "is_private_email": "false",
	}
	mutate(claims)
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = kid
	out, err := t.SignedString(s.key)
	if err != nil {
		panic(err)
	}
	return out
}

func (s *suite) signIn(token string, extra map[string]any) (int, map[string]any, error) {
	body := map[string]any{"identity_token": token}
	for k, v := range extra {
		body[k] = v
	}
	return s.api.JSON("POST", "/api/auth/apple-auth", "", body)
}

func (s *suite) sub(label string) string { return fmt.Sprintf("%d.%s.test", s.run, label) }

func main() {
	base := flag.String("base-url", "http://localhost:8080", "API base URL")
	dsn := flag.String("db", os.Getenv("APPLETEST_DB"), "the server's local Postgres DSN")
	keysAddr := flag.String("keys-addr", "127.0.0.1:8099", "where to serve the signing key")
	flag.Parse()
	if strings.Contains(*base, "tasksy.co.uk") {
		fmt.Println("refusing to run against production")
		os.Exit(2)
	}
	if !strings.Contains(*dsn, "127.0.0.1") && !strings.Contains(*dsn, "localhost") {
		fmt.Println("-db must be a local database")
		os.Exit(2)
	}
	g, err := gorm.Open(postgres.Open(*dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Println("cannot open -db:", err)
		os.Exit(2)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	ln, err := net.Listen("tcp", *keysAddr)
	if err != nil {
		fmt.Println("cannot serve keys:", err)
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	go http.Serve(ln, mux)

	s := &suite{api: newAPI(*base), r: &Runner{}, g: g, key: key, run: time.Now().Unix()}
	fmt.Printf("%sSign in with Apple test%s base=%s run=%d\n", colorBold, colorReset, *base, s.run)
	s.scenarios()
	s.r.Summary()
	os.Exit(s.r.ExitCode())
}

func (s *suite) scenarios() {
	var firstID string
	sub1, mail1 := s.sub("one"), fmt.Sprintf("apple-%d-one@example.com", s.run)

	s.r.Section("Request handling")
	s.r.Check("an empty body is refused (400)", func() (string, error) {
		st, _, err := s.api.JSON("POST", "/api/auth/apple-auth", "", map[string]any{})
		return fmt.Sprint(st), want(st, 400, err)
	})
	s.r.Check("a garbage token is refused (401)", func() (string, error) {
		st, _, err := s.signIn("not.a.token", nil)
		return fmt.Sprint(st), want(st, 401, err)
	})

	s.r.Section("First sign-in creates the account")
	s.r.Check("a new Apple user is created and gets tokens", func() (string, error) {
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }),
			map[string]any{"first_name": "Amy", "last_name": "Apple"})
		if err := want(st, 200, err); err != nil {
			return "", fmt.Errorf("%w: %s", err, errMsg(resp))
		}
		firstID = str(resp, "user", "id")
		if firstID == "" || str(resp, "tokens", "access_token") == "" || str(resp, "tokens", "refresh_token") == "" {
			return "", fmt.Errorf("missing id or tokens: %v", resp)
		}
		if str(resp, "user", "first_name") != "Amy" || str(resp, "user", "last_name") != "Apple" || str(resp, "user", "email") != mail1 {
			return "", fmt.Errorf("profile wrong: %v", resp["user"])
		}
		if dig(resp, "user", "email_verified") != true {
			return "", fmt.Errorf("email should be verified (Apple verified it)")
		}
		return firstID, nil
	})
	s.r.Check("the response never contains a password or Apple ID", func() (string, error) {
		_, resp, _ := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }), nil)
		b, _ := json.Marshal(resp["user"])
		if strings.Contains(string(b), "password") || strings.Contains(string(b), "apple_id") || strings.Contains(string(b), sub1) {
			return "", fmt.Errorf("leak: %s", b)
		}
		return "clean", nil
	})
	s.r.Check("the access token works on a protected endpoint", func() (string, error) {
		_, resp, _ := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }), nil)
		st, _, err := s.api.JSON("GET", "/api/v1/notifications/preferences", str(resp, "tokens", "access_token"), nil)
		return fmt.Sprint(st), want(st, 200, err)
	})

	s.r.Section("Signing in again")
	s.r.Check("same Apple ID returns the same account; names sent again are ignored", func() (string, error) {
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }),
			map[string]any{"first_name": "Mallory", "last_name": "Other"})
		if err := want(st, 200, err); err != nil {
			return "", err
		}
		if str(resp, "user", "id") != firstID || str(resp, "user", "first_name") != "Amy" {
			return "", fmt.Errorf("got %v", resp["user"])
		}
		return "same account", nil
	})
	s.r.Check("a later token without an email still signs in", func() (string, error) {
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"] = sub1 }), nil)
		if err := want(st, 200, err); err != nil {
			return "", err
		}
		return str(resp, "user", "id"), nilIf(str(resp, "user", "id") == firstID, "different account")
	})
	s.r.Check("the account follows the Apple ID when the email on the token changes", func() (string, error) {
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, "changed-"+mail1 }), nil)
		if err := want(st, 200, err); err != nil {
			return "", err
		}
		return str(resp, "user", "email"), nilIf(str(resp, "user", "id") == firstID && str(resp, "user", "email") == mail1, "wrong account or email overwritten")
	})
	s.r.Check("an Apple-only account can't use password login, and is told why", func() (string, error) {
		st, resp, err := s.api.JSON("POST", "/api/auth/login", "", map[string]any{"email": mail1, "password": "Password123"})
		if err := want(st, 400, err); err != nil {
			return "", err
		}
		return errMsg(resp), nilIf(strings.Contains(errMsg(resp), "Apple"), "message should mention Apple")
	})

	s.r.Section("Linking to an existing account")
	pwMail := fmt.Sprintf("apple-%d-pw@example.com", s.run)
	var pwID string
	s.r.MustCheck("an existing password user is registered", func() (string, error) {
		st, resp, err := s.api.Form("/api/auth/register", "", url.Values{
			"first_name": {"Pat"}, "last_name": {"Password"}, "email": {pwMail}, "password": {"Password123"},
			"phone_number": {fmt.Sprintf("+4478%08d", s.run%100000000)},
		})
		if err := want(st, 200, err); err != nil && st != 201 {
			return "", err
		}
		pwID = str(resp, "user", "id")
		return pwID, nil
	})
	s.r.Check("an unverified Apple email can't claim an existing account (403)", func() (string, error) {
		st, _, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"], c["email_verified"] = s.sub("unv"), pwMail, "false" }), nil)
		return fmt.Sprint(st), want(st, 403, err)
	})
	s.r.Check("a verified Apple email links to the existing account", func() (string, error) {
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = s.sub("pw"), strings.ToUpper(pwMail[:3])+pwMail[3:] }), nil)
		if err := want(st, 200, err); err != nil {
			return "", err
		}
		return str(resp, "user", "id"), nilIf(str(resp, "user", "id") == pwID, "created a duplicate instead of linking (email match must ignore case)")
	})
	s.r.Check("the password still works after linking", func() (string, error) {
		st, _, err := s.api.JSON("POST", "/api/auth/login", "", map[string]any{"email": pwMail, "password": "Password123"})
		return fmt.Sprint(st), want(st, 200, err)
	})
	s.r.Check("a different Apple ID can't take over the linked account (409)", func() (string, error) {
		st, _, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = s.sub("thief"), pwMail }), nil)
		return fmt.Sprint(st), want(st, 409, err)
	})

	s.r.Section("Private relay and missing email")
	s.r.Check("a private relay address creates an account", func() (string, error) {
		relay := fmt.Sprintf("x%d@privaterelay.appleid.com", s.run)
		st, resp, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"], c["is_private_email"] = s.sub("relay"), relay, "true" }), nil)
		if err := want(st, 200, err); err != nil {
			return "", err
		}
		return str(resp, "user", "email"), nilIf(str(resp, "user", "email") == relay, "wrong email")
	})
	s.r.Check("a brand-new user whose token has no email is refused (400)", func() (string, error) {
		st, _, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"] = s.sub("noemail") }), nil)
		return fmt.Sprint(st), want(st, 400, err)
	})

	s.r.Section("Token checks")
	for _, c := range []struct {
		name string
		mut  func(jwt.MapClaims)
	}{
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = "com.evil.app" }},
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://accounts.google.com" }},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
	} {
		c := c
		s.r.Check(c.name+" is refused (401)", func() (string, error) {
			st, _, err := s.signIn(s.token(func(m jwt.MapClaims) { m["sub"], m["email"] = s.sub("bad"), "bad@example.com"; c.mut(m) }), nil)
			return fmt.Sprint(st), want(st, 401, err)
		})
	}
	s.r.Check("a token signed with another key is refused (401)", func() (string, error) {
		other, _ := rsa.GenerateKey(rand.Reader, 2048)
		t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "https://appleid.apple.com", "aud": aud, "sub": s.sub("forged"), "email": "forged@example.com", "exp": time.Now().Add(time.Hour).Unix()})
		t.Header["kid"] = kid
		forged, _ := t.SignedString(other)
		st, _, err := s.signIn(forged, nil)
		return fmt.Sprint(st), want(st, 401, err)
	})
	s.r.Check("nonce: the raw value or its hash matches; a wrong one is refused", func() (string, error) {
		nonce := "n-" + fmt.Sprint(s.run)
		tok := s.token(func(c jwt.MapClaims) {
			c["sub"], c["email"], c["nonce"] = s.sub("nonce"), fmt.Sprintf("apple-%d-nonce@example.com", s.run), sha256hex(nonce)
		})
		if st, _, err := s.signIn(tok, map[string]any{"nonce": nonce}); want(st, 200, err) != nil {
			return "", fmt.Errorf("right nonce: status %d", st)
		}
		if st, _, err := s.signIn(tok, map[string]any{"nonce": "wrong"}); want(st, 401, err) != nil {
			return "", fmt.Errorf("wrong nonce: status %d", st)
		}
		return "ok", nil
	})
	s.r.Check("a forged token for an existing Apple ID is refused, so the account stays safe", func() (string, error) {
		other, _ := rsa.GenerateKey(rand.Reader, 2048)
		t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "https://appleid.apple.com", "aud": aud, "sub": sub1, "exp": time.Now().Add(time.Hour).Unix()})
		t.Header["kid"] = kid
		forged, _ := t.SignedString(other)
		st, _, err := s.signIn(forged, nil)
		return fmt.Sprint(st), want(st, 401, err)
	})

	s.r.Section("Accounts that must not sign in")
	s.r.Check("a disabled account is refused (403)", func() (string, error) {
		if err := s.g.Exec(`UPDATE users SET disabled = true WHERE id = ?`, firstID).Error; err != nil {
			return "", err
		}
		defer s.g.Exec(`UPDATE users SET disabled = false WHERE id = ?`, firstID)
		st, _, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }), nil)
		return fmt.Sprint(st), want(st, 403, err)
	})
	s.r.Check("an admin account is refused (403): admins use the admin login", func() (string, error) {
		if err := s.g.Exec(`INSERT INTO roles (name) VALUES ('admin') ON CONFLICT DO NOTHING`).Error; err != nil {
			return "", err
		}
		if err := s.g.Exec(`INSERT INTO user_roles (user_id, role_name) VALUES (?, 'admin') ON CONFLICT DO NOTHING`, firstID).Error; err != nil {
			return "", err
		}
		defer s.g.Exec(`DELETE FROM user_roles WHERE user_id = ? AND role_name = 'admin'`, firstID)
		st, _, err := s.signIn(s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub1, mail1 }), nil)
		return fmt.Sprint(st), want(st, 403, err)
	})

	s.r.Section("Concurrency")
	s.r.Check("five simultaneous first sign-ins make one account", func() (string, error) {
		sub, mail := s.sub("race"), fmt.Sprintf("apple-%d-race@example.com", s.run)
		tok := s.token(func(c jwt.MapClaims) { c["sub"], c["email"] = sub, mail })
		ids, codes := make([]string, 5), make([]int, 5)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				st, resp, _ := s.signIn(tok, nil)
				codes[i], ids[i] = st, str(resp, "user", "id")
			}(i)
		}
		wg.Wait()
		var n int64
		s.g.Raw(`SELECT COUNT(*) FROM users WHERE apple_id = ?`, sub).Scan(&n)
		for i := range ids {
			if codes[i] != 200 || ids[i] != ids[0] {
				return "", fmt.Errorf("codes %v ids %v", codes, ids)
			}
		}
		return fmt.Sprintf("%d row", n), nilIf(n == 1, fmt.Sprintf("%d rows", n))
	})
}

func want(got, want int, err error) error {
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("status %d, want %d", got, want)
	}
	return nil
}

func nilIf(ok bool, msg string) error {
	if ok {
		return nil
	}
	return fmt.Errorf("%s", msg)
}
