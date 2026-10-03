package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Labels the status endpoint uses for fields POST /api/user/stripe-connect-status submits.
var submittableLabels = []string{"Add your address", "Add your legal first name", "Add your legal last name", "Add a valid phone number"}

func (s *Suite) connectStatus(u *TestUser) (map[string]any, error) {
	status, resp, err := s.api.Do("GET", "/api/user/stripe-connect-status", u.AccessToken, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := expect(status, resp, 200); err != nil {
		return nil, err
	}
	return getMap(resp, "data"), nil
}

// onboard takes a user's Connect account from restricted to transfers-active
// through the app's own endpoints, so a broken endpoint fails here instead of
// being bypassed. Only identity documents go to Stripe directly — the app has
// no upload endpoint for them.
func (s *Suite) onboard(u *TestUser, first, last string, otherUserPhone string) {
	p := u.Label + ": "

	s.r.Check(p+"connect status starts action_required", func() (string, error) {
		st, err := s.connectStatus(u)
		if err != nil {
			return "", err
		}
		if getStr(st, "status") == "ready" || getBool(st, "payouts_enabled") {
			return "", fmt.Errorf("brand-new account already ready: %v", st)
		}
		if getStr(st, "account_id") != u.StripeAccountID {
			return "", fmt.Errorf("account_id=%s, expected %s", getStr(st, "account_id"), u.StripeAccountID)
		}
		return fmt.Sprintf("status=%s missing=%v", getStr(st, "status"), getSlice(st, "missing")), nil
	})

	s.r.Check(p+"submit requirements with empty body is refused", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/user/stripe-connect-status", u.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "no details")
	})

	s.r.Check(p+"submit requirements with invalid phone is rejected", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/user/stripe-connect-status", u.AccessToken, map[string]any{"phone_number": "12345"}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "phone")
	})

	if otherUserPhone != "" {
		s.r.Check(p+"submit requirements with another user's phone is refused", func() (string, error) {
			status, resp, err := s.api.Do("POST", "/api/user/stripe-connect-status", u.AccessToken, map[string]any{"phone_number": otherUserPhone}, nil)
			if err != nil {
				return "", err
			}
			return expectRefused(status, resp, "phone")
		})
	}

	s.r.MustCheck(p+"set date of birth via /api/user/update", func() (string, error) {
		status, resp, err := s.api.PostForm("/api/user/update", u.AccessToken, url.Values{"dob": {"1901-01-01"}}, nil)
		if err != nil {
			return "", err
		}
		return "dob=1901-01-01 (stripe test auto-verify value)", expect(status, resp, 200)
	})

	phone := randomUKPhone()
	s.r.Check(p+"submit name/phone/address clears those requirements", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/user/stripe-connect-status", u.AccessToken, map[string]any{
			"first_name":   first,
			"last_name":    last,
			"phone_number": phone,
			"address": map[string]any{
				"line1": "10 Downing Street", "line2": "", "city": "London",
				"state": "", "postal_code": "SW1A 2AA", "country": "GB",
			},
		}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		// The response is the refreshed status; Stripe can lag a moment, so
		// re-read until the submitted fields are gone.
		return pollUntil(20*time.Second, func() (string, bool, error) {
			st, err := s.connectStatus(u)
			if err != nil {
				return "", false, err
			}
			var still []string
			for _, m := range getSlice(st, "missing") {
				for _, label := range submittableLabels {
					if m == label {
						still = append(still, m)
					}
				}
			}
			if len(still) > 0 {
				return "still missing after submit: " + strings.Join(still, ", "), false, nil
			}
			return fmt.Sprintf("remaining=%v", getSlice(st, "missing")), true, nil
		})
	})
	u.Phone = phone

	s.r.MustCheck(p+"add bank account (stripe test UK bank)", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/bank-account", u.AccessToken, map[string]any{
			"account_holder_name": first + " " + last,
			"sort_code":           "108800",
			"account_number":      "00012345",
		}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		u.BankID = getStr(resp, "data", "id")
		u.BankStripeID = getStr(resp, "data", "stripe_bank_account_id")
		if u.BankStripeID == "" || !getBool(resp, "data", "is_default") {
			return "", fmt.Errorf("bank not attached on stripe or not default: %v", resp)
		}
		return "bank=" + u.BankStripeID + " default=true", nil
	})

	var personID string
	s.r.MustCheck(p+"find stripe person", func() (string, error) {
		acc, err := s.sc.Get("/accounts/"+u.StripeAccountID, "")
		if err != nil {
			return "", err
		}
		personID = getStr(acc, "individual", "id")
		if personID == "" {
			return "", fmt.Errorf("account has no individual person")
		}
		return personID, nil
	})

	// Both documents up front: the keyed-identity check can fail even when
	// the primary document passes, and additional_document is Stripe's
	// documented alternate path for that case.
	s.r.MustCheck(p+"upload identity documents (stripe test tokens)", func() (string, error) {
		_, err := s.sc.Post("/accounts/"+u.StripeAccountID+"/persons/"+personID, url.Values{
			"verification[document][front]":            {"file_identity_document_success"},
			"verification[additional_document][front]": {"file_identity_document_success"},
		}, "")
		return "document + additional_document submitted", err
	})

	s.r.MustCheck(p+"transfers capability becomes active", func() (string, error) {
		return pollUntil(3*time.Minute, func() (string, bool, error) {
			acc, err := s.sc.Get("/accounts/"+u.StripeAccountID, "")
			if err != nil {
				return "", false, err
			}
			tr := getStr(acc, "capabilities", "transfers")
			return fmt.Sprintf("transfers=%s due=%v", tr, getSlice(acc, "requirements", "currently_due")), tr == "active", nil
		})
	})

	s.r.Check(p+"connect status endpoint reports ready", func() (string, error) {
		return pollUntil(60*time.Second, func() (string, bool, error) {
			st, err := s.connectStatus(u)
			if err != nil {
				return "", false, err
			}
			return fmt.Sprintf("status=%s missing=%v", getStr(st, "status"), getSlice(st, "missing")), getStr(st, "status") == "ready", nil
		})
	})
}
