package main

import (
	"fmt"
	"net/url"
	"time"
)

func (s *Suite) preflight() {
	s.r.Section("Preflight")
	s.r.MustCheck("server responds on /ping with system settings", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/ping", "", nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		s.settings = parseSettings(getMap(resp, "settings"))
		return fmt.Sprintf("client=%.2f%% freelancer=%.2f%% platform_fee=£%.2f",
			s.settings.ClientCommissionPct, s.settings.FreelancerCommissionPct, s.settings.PlatformFee), nil
	})
	s.r.Check("system settings are non-zero (fee math is meaningful)", func() (string, error) {
		if s.settings.ClientCommissionPct == 0 && s.settings.FreelancerCommissionPct == 0 && s.settings.PlatformFee == 0 {
			return "", fmt.Errorf("all fees are zero — seed system_settings")
		}
		return "ok", nil
	})
	s.r.MustCheck("stripe test key works", func() (string, error) {
		bal, err := s.sc.Get("/balance", "")
		if err != nil {
			return "", err
		}
		if getBool(bal, "livemode") {
			return "", fmt.Errorf("key is live mode")
		}
		return "livemode=false", nil
	})
}

func (s *Suite) setupUsers(client, f1, f2 *TestUser) {
	s.r.Section("Users")
	for _, u := range []struct {
		user        *TestUser
		first, last string
	}{{client, "Test", "Client"}, {f1, "Test", "Freelancer"}, {f2, "Test", "Freelancer2"}} {
		u := u
		s.r.MustCheck("register "+u.user.Label, func() (string, error) { return s.register(u.user, u.first, u.last) })
	}

	if s.db != nil {
		s.admin = &TestUser{Label: "admin", Email: "paytest-admin@example.com", Password: "Password123", Ledger: newLedger()}
		s.r.MustCheck("provision admin user (via -db)", func() (string, error) {
			if _, err := s.login(s.admin); err != nil {
				if _, err := s.register(s.admin, "PayTest", "Admin"); err != nil {
					return "", err
				}
			}
			if err := s.db.grantAdmin(s.admin.ID); err != nil {
				return "", err
			}
			_, err := s.login(s.admin)
			s.adminToken = s.admin.AccessToken
			return s.admin.Email, err
		})
	}
	if s.adminToken != "" {
		s.r.MustCheck("admin token passes the admin role check", func() (string, error) {
			status, resp, err := s.api.Do("GET", "/api/v3/admin/payouts/escrows?limit=1", s.adminToken, nil, nil)
			if err != nil {
				return "", err
			}
			return "200", expect(status, resp, 200)
		})
	}

	s.r.MustCheck("create test category", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v1/category/create", "", map[string]any{"name": fmt.Sprintf("PayTest-%d", s.run)}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		s.catID = getStr(resp, "data", "id")
		return s.catID, nil
	})
	// Client is verified too, so the "can't bid on your own job" rule is reachable.
	s.r.MustCheck("verify identity: client", func() (string, error) { return s.verifyIdentity(client) })
	s.r.MustCheck("verify identity: freelancer", func() (string, error) { return s.verifyIdentity(f1) })
}

// unauthenticatedAccess: money endpoints must reject anonymous callers.
func (s *Suite) unauthenticatedAccess(someone *TestUser) {
	s.r.Section("Access control — anonymous and non-admin callers")
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v3/wallet"},
		{"GET", "/api/v3/wallet/payout-balance"},
		{"POST", "/api/v3/wallet/withdraw"},
		{"POST", "/api/v3/payments/intent"},
		{"GET", "/api/v3/payments/transactions"},
		{"POST", "/api/v3/escrow/contracts/x/release"},
		{"GET", "/api/v3/bank-account"},
		{"GET", "/api/user/stripe-connect-status"},
		{"GET", "/api/v3/admin/payouts/escrows"},
		{"GET", "/api/v3/admin/payments/transactions"},
		{"GET", "/api/v3/admin/payments/transactions/x"},
		{"GET", "/api/v3/admin/payments/stats"},
		{"GET", "/api/v1/admin/users"},
		{"GET", "/api/v1/admin/users/" + someone.ID},
		{"GET", "/api/v1/admin/users/" + someone.ID + "/wallet"},
	} {
		ep := ep
		s.r.Check("anonymous "+ep.method+" "+ep.path+" is rejected", func() (string, error) {
			status, resp, err := s.api.Do(ep.method, ep.path, "", nil, nil)
			if err != nil {
				return "", err
			}
			if status != 401 && status != 403 {
				return "", fmt.Errorf("got %d — endpoint is open to anyone: %s", status, brief(resp))
			}
			return fmt.Sprintf("%d", status), nil
		})
	}
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v3/admin/payouts/escrows"},
		{"GET", "/api/v3/admin/payouts/withdrawals"},
		{"POST", "/api/v3/admin/payouts/contracts/x/refund"},
		{"POST", "/api/v3/admin/payouts/escrows/x/release"},
	} {
		ep := ep
		s.r.Check("non-admin "+ep.method+" "+ep.path+" is 403", func() (string, error) {
			status, resp, err := s.api.Do(ep.method, ep.path, someone.AccessToken, map[string]any{"reason": "x", "note": "x"}, nil)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d", status), expect(status, resp, 403)
		})
	}
}

func (s *Suite) bankAccounts(f1, client *TestUser) {
	s.r.Section("Bank accounts")
	s.r.Check("invalid sort code is rejected", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/bank-account", f1.AccessToken, map[string]any{
			"account_holder_name": "Test Freelancer", "sort_code": "000000", "account_number": "1",
		}, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			return "", fmt.Errorf("invalid bank details accepted: %s", brief(resp))
		}
		return fmt.Sprintf("%d: %s", status, errMsg(resp)), nil
	})
	s.r.Check("missing fields are rejected", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/bank-account", f1.AccessToken, map[string]any{"sort_code": "108800"}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})

	var secondID string
	s.r.Check("second bank account is added as non-default", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/bank-account", f1.AccessToken, map[string]any{
			"account_holder_name": "Test Freelancer", "sort_code": "108800", "account_number": "00012345",
		}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		secondID = getStr(resp, "data", "id")
		if getBool(resp, "data", "is_default") {
			return "", fmt.Errorf("second bank account became default")
		}
		return secondID, nil
	})
	s.r.Check("bank list shows both, default first", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v3/bank-account", f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		rows := getRows(resp, "data")
		if len(rows) != 2 || getStr(rows[0], "id") != f1.BankID || !getBool(rows[0], "is_default") {
			return "", fmt.Errorf("unexpected list: %d rows, first=%s", len(rows), getStr(rows[0], "id"))
		}
		return "2 accounts, default first", nil
	})
	s.r.Check("another user cannot delete this bank account", func() (string, error) {
		status, resp, err := s.api.Do("DELETE", "/api/v3/bank-account/"+secondID, client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 404)
	})
	s.r.Check("delete non-default bank account", func() (string, error) {
		status, resp, err := s.api.Do("DELETE", "/api/v3/bank-account/"+secondID, f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		return "deleted", expect(status, resp, 200)
	})
	s.r.Check("stripe still has the default bank attached", func() (string, error) {
		acc, err := s.sc.Get("/accounts/"+f1.StripeAccountID, "")
		if err != nil {
			return "", err
		}
		for _, ext := range getRows(acc, "external_accounts", "data") {
			if getStr(ext, "id") == f1.BankStripeID && getBool(ext, "default_for_currency") {
				return f1.BankStripeID + " default_for_currency=true", nil
			}
		}
		return "", fmt.Errorf("default bank %s missing on stripe", f1.BankStripeID)
	})
}

func (s *Suite) contractCount(token, proposalID string) (int, error) {
	status, resp, err := s.api.Do("GET", "/api/contracts/list?proposal_id="+url.QueryEscape(proposalID), token, nil, nil)
	if err != nil {
		return 0, err
	}
	if err := expect(status, resp, 200); err != nil {
		return 0, err
	}
	return int(getNum(resp, "meta", "total")), nil
}

func (s *Suite) proposalAndContractRules(client, f1, f2 *TestUser) {
	s.r.Section("Job, proposal and contract rules")
	const budget = 20.0
	fees := computeFees(budget, s.settings)

	var jobID string
	s.r.MustCheck("client posts job", func() (string, error) {
		id, err := s.createJob(client, "paytest rules", budget)
		jobID = id
		return id, err
	})
	s.r.Check("job payment preview matches fee formula", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/jobs/"+jobID+"/payment-details", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if got := getNum(resp, "data", "grand_total"); !eq(got, fees.ClientTotal) {
			return "", fmt.Errorf("grand_total=%.2f want %.2f", got, fees.ClientTotal)
		}
		if got := getNum(resp, "data", "payment_summary", "freelancer", "amount_to_receive"); !eq(got, fees.FreelancerNet) {
			return "", fmt.Errorf("freelancer amount_to_receive=%.2f want %.2f", got, fees.FreelancerNet)
		}
		return fmt.Sprintf("client pays £%.2f, freelancer gets £%.2f", fees.ClientTotal, fees.FreelancerNet), nil
	})

	refused := func(name string, who *TestUser, extra map[string]any, contains string) {
		s.r.Check(name, func() (string, error) {
			status, resp, err := s.sendProposal(who, jobID, budget, extra)
			if err != nil {
				return "", err
			}
			return expectRefused(status, resp, contains)
		})
	}
	refused("unverified freelancer cannot propose", f2, nil, "verified")
	refused("missing cover_letter is refused by name", f1, map[string]any{"cover_letter": nil}, "cover_letter")
	refused("wrong-typed bid_amount is refused by name", f1, map[string]any{"bid_amount": "ten"}, "bid_amount")
	refused("zero bid is refused", f1, map[string]any{"bid_amount": 0}, "bid_amount")
	refused("bid above fixed budget is refused", f1, map[string]any{"bid_amount": budget + 5}, "exceeds")
	refused("bad availability_date format is refused", f1, map[string]any{"availability_date": "01/12/2026"}, "availability_date")
	refused("availability end before start is refused", f1, map[string]any{
		"availability_date": "2026-12-02 10:00:00", "availability_date_end": "2026-12-01 10:00:00",
	}, "before")
	refused("client cannot bid on own job", client, nil, "own job")
	s.r.Check("empty request body is refused clearly", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/job/send-proposal", f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "required")
	})
	s.r.Check("proposal on unknown job is refused", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/job/send-proposal", f1.AccessToken, map[string]any{
			"job_post_id": "00000000-0000-0000-0000-000000000000", "cover_letter": "x", "bid_amount": 5, "duration": 1,
		}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "not found")
	})

	var proposalID string
	s.r.MustCheck("valid proposal with 'YYYY-MM-DD HH:MM:SS' dates is accepted", func() (string, error) {
		status, resp, err := s.sendProposal(f1, jobID, budget, map[string]any{
			"availability_date": "2026-12-01 15:15:00", "availability_date_end": "2026-12-01 16:20:00",
		})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		proposalID = getStr(resp, "data", "id")
		return proposalID, nil
	})
	refused("duplicate proposal on same job is refused", f1, nil, "already")

	s.r.Check("proposal payment summary matches fee formula", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/proposals/"+proposalID+"/payment-summary", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if got := getNum(resp, "data", "grand_total"); !eq(got, fees.ClientTotal) {
			return "", fmt.Errorf("grand_total=%.2f want %.2f", got, fees.ClientTotal)
		}
		if got := getNum(resp, "data", "payment_summary", "platform_earnings"); !eq(got, fees.PlatformEarnings) {
			return "", fmt.Errorf("platform_earnings=%.2f want %.2f", got, fees.PlatformEarnings)
		}
		return fmt.Sprintf("total £%.2f, platform £%.2f", fees.ClientTotal, fees.PlatformEarnings), nil
	})
	s.r.Check("outsider cannot view the proposal", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/proposals/"+proposalID, f2.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})

	// Money must not move before the client has accepted the proposal.
	s.r.Check("payment intent for a pending (unaccepted) proposal is refused", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, proposalID, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})
	s.r.Check("contract for a pending (unaccepted) proposal is refused and not created", func() (string, error) {
		status, resp, err := s.createContract(client.AccessToken, proposalID, budget, time.Now(), time.Now().Add(24*time.Hour))
		if err != nil {
			return "", err
		}
		n, cerr := s.contractCount(client.AccessToken, proposalID)
		if cerr != nil {
			return "", cerr
		}
		if n != 0 {
			return "", fmt.Errorf("responded %d but a contract was created anyway (%d found)", status, n)
		}
		return expectRefused(status, resp, "")
	})

	s.r.Check("outsider cannot accept the proposal", func() (string, error) {
		status, resp, err := s.decide(f2.AccessToken, proposalID, "accepted")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})
	s.r.Check("invalid decision value is refused", func() (string, error) {
		status, resp, err := s.decide(client.AccessToken, proposalID, "maybe")
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})
	s.r.Check("client rejects proposal", func() (string, error) {
		status, resp, err := s.decide(client.AccessToken, proposalID, "rejected")
		if err != nil {
			return "", err
		}
		return getStr(resp, "status"), expect(status, resp, 200)
	})
	s.r.Check("rejected proposal cannot then be accepted", func() (string, error) {
		status, resp, err := s.decide(client.AccessToken, proposalID, "accepted")
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})

	// A second job for the contract-creation rules on an accepted proposal.
	var job2, proposal2 string
	s.r.MustCheck("second job with an accepted proposal", func() (string, error) {
		id, err := s.createJob(client, "paytest contract rules", budget)
		if err != nil {
			return "", err
		}
		job2 = id
		status, resp, err := s.sendProposal(f1, job2, budget, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		proposal2 = getStr(resp, "data", "id")
		status, resp, err = s.decide(client.AccessToken, proposal2, "accepted")
		if err != nil {
			return "", err
		}
		return proposal2, expect(status, resp, 200)
	})
	s.inProgressJobID = job2
	s.r.Check("non-owner cannot create the contract", func() (string, error) {
		status, resp, err := s.createContract(f1.AccessToken, proposal2, budget, time.Now(), time.Now().Add(24*time.Hour))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})
	s.r.Check("contract with end_date before start_date is refused and not created", func() (string, error) {
		status, resp, err := s.createContract(client.AccessToken, proposal2, budget, time.Now().Add(48*time.Hour), time.Now())
		if err != nil {
			return "", err
		}
		n, cerr := s.contractCount(client.AccessToken, proposal2)
		if cerr != nil {
			return "", cerr
		}
		if n != 0 {
			return "", fmt.Errorf("responded %d but a contract was created anyway", status)
		}
		return expectRefused(status, resp, "")
	})
}

// webhookForgery: the webhook must only trust Stripe-signed payloads.
func (s *Suite) webhookForgery(client *TestUser) {
	s.r.Section("Webhook security")
	forged := []byte(`{"id":"evt_forged_paymenttest","type":"charge.succeeded","data":{"object":{"id":"ch_forged","amount":100,"currency":"gbp","metadata":{"proposal_id":"forged"}}}}`)
	s.r.Check("unsigned webhook is rejected", func() (string, error) {
		status, resp, err := s.api.DoRaw("/api/v3/webhooks/stripe/payment", forged, map[string]string{"Content-Type": "application/json"})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 400)
	})
	s.r.Check("webhook with a forged signature is rejected", func() (string, error) {
		sig := fmt.Sprintf("t=%d,v1=%064d", time.Now().Unix(), 0)
		status, resp, err := s.api.DoRaw("/api/v3/webhooks/stripe/payment", forged, map[string]string{
			"Content-Type": "application/json", "Stripe-Signature": sig,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 400)
	})
}
