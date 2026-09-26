package main

import (
	"fmt"
	"net/url"
	"time"
)


func onboardStripeConnect(api *APIClient, sc *StripeClient, r *Runner, user *TestUser, label, accountHolderName string) {
	r.Check(label+": connect account starts restricted", func() (string, error) {
		acc, err := sc.Get("/accounts/"+user.StripeAccountID, "")
		if err != nil {
			return "", err
		}
		if getBool(acc, "charges_enabled") {
			return "", fmt.Errorf("expected a brand-new account to be restricted, but charges_enabled is already true")
		}
		return fmt.Sprintf("charges_enabled=false, disabled_reason=%v", getStr(acc, "requirements", "disabled_reason")), nil
	})

	r.MustCheck(label+": set dob (magic test value for auto-verification)", func() (string, error) {
		form := url.Values{"dob": {"1901-01-01"}}
		status, resp, err := api.PostForm("/api/user/update", user.AccessToken, form, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		return "dob=1901-01-01", nil
	})

	r.MustCheck(label+": add address", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v1/user/"+user.ID+"/addresses", user.AccessToken, map[string]any{
			"line1": "10 Downing Street", "city": "London", "state": "London",
			"postal_code": "SW1A 2AA", "country": "GB", "is_default": true,
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		return "address saved", nil
	})

	r.MustCheck(label+": add bank account (stripe test UK bank)", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v3/bank-account", user.AccessToken, map[string]any{
			"account_holder_name": accountHolderName,
			"sort_code":           "108800",
			"account_number":      "00012345",
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		data := getMap(resp, "data")
		bankID := getStr(data, "stripe_bank_account_id")
		if bankID == "" {
			return "", fmt.Errorf("no stripe_bank_account_id in response: %v", resp)
		}
		return "bank=" + getStr(data, "bank_name") + " last4=" + getStr(data, "account_number_last4"), nil
	})

	r.MustCheck(label+": re-login to push profile to stripe", func() (string, error) {
		return loginUser(api, user)
	})

	// provisionStripeAccount runs in a fire-and-forget goroutine on the server,
	// so the sync isn't done the instant login returns — poll for it.
	var personID string
	r.MustCheck(label+": stripe account phone/name/address requirements satisfied", func() (string, error) {
		return pollUntil(15*time.Second, func() (string, bool, error) {
			acc, err := sc.Get("/accounts/"+user.StripeAccountID, "")
			if err != nil {
				return "", false, err
			}
			individual := getMap(acc, "individual")
			personID = getStr(individual, "id")
			due := getSlice(acc, "requirements", "currently_due")
			remaining := filterOut(due, "external_account")
			if len(remaining) > 0 {
				return fmt.Sprintf("still due: %v", remaining), false, nil
			}
			return "external_account satisfied by bank; no other currently_due fields", true, nil
		})
	})

	// GB phone numbers need a real, valid-looking number; the app's own phone
	// (a UK "drama" test number) isn't accepted by Stripe's person.phone
	// validator, so fix it directly to isolate the identity check. Each user
	// needs a distinct number — Stripe rejects reusing one across persons.
	r.MustCheck(label+": set a stripe-valid phone number on the person", func() (string, error) {
		phone := randomUKPhone()
		form := url.Values{"phone": {phone}}
		_, err := sc.Post("/accounts/"+user.StripeAccountID+"/persons/"+personID, form, "")
		return "phone=" + phone, err
	})

	r.MustCheck(label+": simulate successful identity document upload (test-mode magic token)", func() (string, error) {
		form := url.Values{"verification[document][front]": {"file_identity_document_success"}}
		_, err := sc.Post("/accounts/"+user.StripeAccountID+"/persons/"+personID, form, "")
		return "verification.document.front=file_identity_document_success", err
	})

	// The keyed-identity check (does the typed name/address/dob match a real
	// identity?) can fail even after document verification succeeds — Stripe's
	// own requirements payload names "verification.additional_document" as the
	// documented alternate path for exactly that case. Submit it upfront rather
	// than reactively: it's harmless if the plain document check would have
	// passed alone, and it avoids racing the exact moment Stripe flips the
	// primary check to "inactive" between polls.
	r.MustCheck(label+": submit additional_document as the alternate verification path", func() (string, error) {
		form := url.Values{"verification[additional_document][front]": {"file_identity_document_success"}}
		_, err := sc.Post("/accounts/"+user.StripeAccountID+"/persons/"+personID, form, "")
		return "verification.additional_document.front=file_identity_document_success", err
	})

	r.Check(label+": wait for capability activation", func() (string, error) {
		return pollUntil(3*time.Minute, func() (string, bool, error) {
			acc, err := sc.Get("/accounts/"+user.StripeAccountID, "")
			if err != nil {
				return "", false, err
			}
			caps := getMap(acc, "capabilities")
			transfers := getStr(caps, "transfers")
			if transfers == "active" {
				return fmt.Sprintf("capabilities=%v charges_enabled=%v", caps, getBool(acc, "charges_enabled")), true, nil
			}
			return "transfers=" + transfers, false, nil
		})
	})
}
