package main

import (
	"fmt"
	"strings"
)

func (s *Suite) adminLoginCall(email, password string) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/auth/admin/login", "", map[string]any{"email": email, "password": password}, nil)
}

func (s *Suite) adminLogin(email, password string) (string, error) {
	status, resp, err := s.adminLoginCall(email, password)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200); err != nil {
		return "", err
	}
	return getStr(resp, "tokens", "access_token"), nil
}

func (s *Suite) adminLoginSecurity(client *TestUser) {
	s.r.Section("Admin login — only administrators, only with the password")
	if s.admin == nil {
		s.r.Skip("admin login checks", "no -db, so no admin test user")
		return
	}

	var unknownMsg string
	s.r.Check("unknown email is refused (401) and gives no token", func() (string, error) {
		status, resp, err := s.adminLoginCall(fmt.Sprintf("nobody-%d@example.com", s.run), "Password123")
		if err != nil {
			return "", err
		}
		unknownMsg = errMsg(resp)
		if getStr(resp, "tokens", "access_token") != "" {
			return "", fmt.Errorf("a token was issued")
		}
		return unknownMsg, expect(status, resp, 401)
	})
	s.r.Check("wrong password gives the same 401 and message as an unknown email", func() (string, error) {
		status, resp, err := s.adminLoginCall(s.admin.Email, "definitely-wrong")
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 401); err != nil {
			return "", err
		}
		if errMsg(resp) != unknownMsg {
			return "", fmt.Errorf("message %q differs from %q, so it reveals which emails exist", errMsg(resp), unknownMsg)
		}
		return "indistinguishable", nil
	})
	s.r.Check("a normal user with the right password is refused (403) and gets no token", func() (string, error) {
		status, resp, err := s.adminLoginCall(client.Email, client.Password)
		if err != nil {
			return "", err
		}
		if getStr(resp, "tokens", "access_token") != "" || strings.Contains(fmt.Sprint(resp), "eyJ") {
			return "", fmt.Errorf("a token was issued to a non-admin")
		}
		return errMsg(resp), expect(status, resp, 403)
	})
	s.r.Check("missing password or email is refused (400)", func() (string, error) {
		for _, body := range []map[string]any{{"email": s.admin.Email}, {"password": "x"}, {}} {
			status, resp, err := s.api.Do("POST", "/api/auth/admin/login", "", body, nil)
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 400); err != nil {
				return "", fmt.Errorf("%v: %w", body, err)
			}
		}
		return "400 for each", nil
	})

	var adminTok string
	s.r.MustCheck("the admin signs in with the right password (email case ignored)", func() (string, error) {
		status, resp, err := s.adminLoginCall(strings.ToUpper(s.admin.Email), s.admin.Password)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		adminTok = getStr(resp, "tokens", "access_token")
		if adminTok == "" || getStr(resp, "tokens", "refresh_token") == "" || getStr(resp, "user", "id") != s.admin.ID {
			return "", fmt.Errorf("incomplete response: %s", brief(resp))
		}
		if body := strings.ToLower(fmt.Sprint(resp)); strings.Contains(body, "password") || strings.Contains(body, "$2a$") {
			return "", fmt.Errorf("response leaks password data")
		}
		return "tokens issued, no password data in the response", nil
	})
	s.r.Check("the admin token opens admin routes; a normal token still doesn't", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v1/admin/users?limit=1", adminTok, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", fmt.Errorf("admin: %w", err)
		}
		status, resp, err = s.api.Do("GET", "/api/v1/admin/users?limit=1", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		return "admin 200, user 403", expect(status, resp, 403)
	})

	s.r.Check("removing the admin role locks the admin out immediately", func() (string, error) {
		if s.db == nil {
			return "skipped (no -db)", nil
		}
		if err := s.db.revokeAdmin(s.admin.ID); err != nil {
			return "", err
		}
		status, resp, err := s.api.Do("GET", "/api/v1/admin/users?limit=1", adminTok, nil, nil)
		if err != nil {
			return "", err
		}
		lockedOut := status == 403
		if err := s.db.grantAdmin(s.admin.ID); err != nil {
			return "", err
		}
		if !lockedOut {
			return "", fmt.Errorf("token still worked after the role was removed: %d %s", status, brief(resp))
		}
		return "403 straight after revoking the role", nil
	})

	s.r.Check("password-less token route refuses an admin by email and by phone", func() (string, error) {
		for _, body := range []map[string]any{{"email": s.admin.Email}, {"phone_number": s.admin.LoginPhone}} {
			status, resp, err := s.api.Do("POST", "/api/auth/user/auth-token", "", body, nil)
			if err != nil {
				return "", err
			}
			if getStr(resp, "tokens", "access_token") != "" {
				return "", fmt.Errorf("a token for the admin was issued without a password (%v)", body)
			}
			if err := expect(status, resp, 403); err != nil {
				return "", err
			}
		}
		return "403, no token", nil
	})
	s.r.Check("reset-password can't change an admin's password", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/auth/reset-password", "", map[string]any{
			"phone_number": s.admin.LoginPhone, "password": "Hijacked12345",
		}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 403); err != nil {
			return "", err
		}
		if _, err := s.adminLogin(s.admin.Email, s.admin.Password); err != nil {
			return "", fmt.Errorf("the admin's own password stopped working: %w", err)
		}
		return "403, original password still works", nil
	})
	s.r.Check("reset-password for an unknown phone is a clean 404", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/auth/reset-password", "", map[string]any{
			"phone_number": "+440000000000", "password": "Whatever12345",
		}, nil)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 404)
	})

	s.r.Check("repeated wrong passwords lock the account out, even for the right password", func() (string, error) {
		victim := newUser("lockout", s.run)
		if _, err := s.register(victim, "Lock", "Out"); err != nil {
			return "", err
		}
		if err := s.db.grantAdmin(victim.ID); err != nil {
			return "", err
		}
		defer func() { _ = s.db.revokeAdmin(victim.ID) }()
		target := victim.Email
		var codes []int
		for i := 0; i < 5; i++ {
			status, _, err := s.adminLoginCall(target, fmt.Sprintf("wrong-%d", i))
			if err != nil {
				return "", err
			}
			codes = append(codes, status)
		}
		status, resp, err := s.adminLoginCall(target, "Password123")
		if err != nil {
			return "", err
		}
		if status != 429 || getStr(resp, "tokens", "access_token") != "" {
			return "", fmt.Errorf("after %v the correct password got %d: %s", codes, status, brief(resp))
		}
		return fmt.Sprintf("failures %v, then 429", codes), nil
	})
	s.r.Check("the lockout doesn't affect other admins", func() (string, error) {
		_, err := s.adminLogin(s.admin.Email, s.admin.Password)
		return "still signs in", err
	})

	s.r.Check("a normal user can still use the normal login", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/auth/login", "", map[string]any{"email": client.Email, "password": client.Password}, nil)
		if err != nil {
			return "", err
		}
		return "200", expect(status, resp, 200)
	})
}
