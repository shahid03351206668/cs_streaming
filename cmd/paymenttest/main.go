//	STRIPE_SECRET_KEY=sk_test_... go run ./cmd/paymenttest [-base-url http://localhost:8080]

package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"time"
)

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "Tasksy API base URL")
	stripeKey := flag.String("stripe-key", os.Getenv("STRIPE_SECRET_KEY"), "Stripe secret key (test mode)")
	adminToken := flag.String("admin-token", os.Getenv("ADMIN_ACCESS_TOKEN"), "access token for a user with the 'admin' role (optional; enables admin-route checks)")
	flag.Parse()

	if *stripeKey == "" {
		fmt.Println("STRIPE_SECRET_KEY is required (env var or -stripe-key)")
		os.Exit(2)
	}
	if len(*stripeKey) < 8 || (*stripeKey)[:8] != "sk_test_" {
		fmt.Println("refusing to run: STRIPE_SECRET_KEY does not look like a test-mode key (must start with sk_test_)")
		os.Exit(2)
	}

	api := NewAPIClient(*baseURL)
	sc := NewStripeClient(*stripeKey)
	r := &Runner{}
	run := int64(time.Now().Unix())
	rand.Seed(run)

	fmt.Printf("Tasksy payment flow validation — base=%s run=%d\n", *baseURL, run)

	r.Section("0. Server health")

	var settings SystemSettings
	r.MustCheck("server responds on /ping", func() (string, error) {
		status, resp, err := api.Do("GET", "/ping", "", nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		settings = parseSettings(getMap(resp, "settings"))
		return fmt.Sprintf("commission client=%.2f%% freelancer=%.2f%% platform_fee=%.2f",
			settings.ClientCommissionPct, settings.FreelancerCommissionPct, settings.PlatformFee), nil
	})

	if settings.ClientCommissionPct == 0 && settings.FreelancerCommissionPct == 0 && settings.PlatformFee == 0 {
		fmt.Printf("%s[WARN]%s system_settings is all zero — fee math will be checked but every fee will be 0. "+
			"Seed non-zero settings to meaningfully validate commission math.\n", colorYellow, colorReset)
	}

	// ---------------------------------------------------------------------
	r.Section("1. User registration")

	client := &TestUser{Email: fmt.Sprintf("paytest-client-%d@example.com", run), Password: "Password123"}
	freelancer := &TestUser{Email: fmt.Sprintf("paytest-freelancer-%d@example.com", run), Password: "Password123"}

	r.MustCheck("register client", func() (string, error) {
		return registerUser(api, client, "Client")
	})
	r.MustCheck("register freelancer", func() (string, error) {
		return registerUser(api, freelancer, "Freelancer")
	})

	r.Check("client has a stripe connect account id", func() (string, error) {
		if client.StripeAccountID == "" {
			return "", fmt.Errorf("stripe_connect_account_id is empty on registration response")
		}
		return client.StripeAccountID, nil
	})
	r.Check("freelancer has a stripe connect account id", func() (string, error) {
		if freelancer.StripeAccountID == "" {
			return "", fmt.Errorf("stripe_connect_account_id is empty on registration response")
		}
		return freelancer.StripeAccountID, nil
	})

	r.MustCheck("mark freelancer identity_verified (admin)", func() (string, error) {
		status, resp, err := api.Do("PUT", "/api/v1/admin/users/"+freelancer.ID, "", map[string]any{
			"identity_verified": true,
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		return "identity_verified=true", nil
	})

	// ---------------------------------------------------------------------
	// Every registered user gets a Stripe Connect account (the app doesn't
	// only create one for freelancers), so both accounts sit "Past due" in
	// the Dashboard until onboarded — clear both, not just the freelancer's,
	// so a test run doesn't leave dangling restricted test accounts behind.
	r.Section("2. Stripe Connect onboarding (client)")
	onboardStripeConnect(api, sc, r, client, "client", "Test Client")

	r.Check("client connect account is now fully enabled", func() (string, error) {
		acc, err := sc.Get("/accounts/"+client.StripeAccountID, "")
		if err != nil {
			return "", err
		}
		if !getBool(acc, "charges_enabled") || !getBool(acc, "payouts_enabled") {
			return "", fmt.Errorf("charges_enabled=%v payouts_enabled=%v", getBool(acc, "charges_enabled"), getBool(acc, "payouts_enabled"))
		}
		return "charges_enabled=true payouts_enabled=true", nil
	})

	r.Section("3. Stripe Connect onboarding (freelancer)")
	onboardStripeConnect(api, sc, r, freelancer, "freelancer", "Test Freelancer")

	r.Check("freelancer connect account is now fully enabled", func() (string, error) {
		acc, err := sc.Get("/accounts/"+freelancer.StripeAccountID, "")
		if err != nil {
			return "", err
		}
		if !getBool(acc, "charges_enabled") || !getBool(acc, "payouts_enabled") {
			return "", fmt.Errorf("charges_enabled=%v payouts_enabled=%v", getBool(acc, "charges_enabled"), getBool(acc, "payouts_enabled"))
		}
		return "charges_enabled=true payouts_enabled=true", nil
	})

	r.Section("4. Job, proposal, contract")

	catID := ""
	r.MustCheck("create test category", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v1/category/create", "", map[string]any{
			"name": fmt.Sprintf("PayTest-%d", run),
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		catID = getStr(resp, "data", "id")
		return catID, nil
	})

	const bidAmount = 5.00
	job := &TestJob{}
	r.MustCheck("client creates job", func() (string, error) {
		id, err := createJob(api, client, catID, "Payment flow test job", bidAmount)
		job.ID = id
		return id, err
	})

	proposal := &TestProposal{}
	r.MustCheck("freelancer submits proposal", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/job/send-proposal", freelancer.AccessToken, map[string]any{
			"job_post_id":  job.ID,
			"cover_letter": "I will complete this job well and on time.",
			"bid_amount":   bidAmount,
			"duration":     1,
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		proposal.ID = getStr(resp, "data", "id")
		if proposal.ID == "" {
			return "", fmt.Errorf("no proposal id in response: %v", resp)
		}
		return proposal.ID, nil
	})

	r.MustCheck("client accepts proposal", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/proposals/"+proposal.ID+"/decision", client.AccessToken, map[string]any{
			"status": "accepted",
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		return getStr(resp, "status"), nil
	})

	contract := &TestContract{}
	r.MustCheck("client creates contract", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/proposals/create-contract", client.AccessToken, map[string]any{
			"proposal_id":  proposal.ID,
			"title":        "Payment flow test job",
			"description":  "created by paymenttest",
			"total_amount": bidAmount,
			"start_date":   time.Now().Format(time.RFC3339),
			"end_date":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"terms":        "Standard",
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		contract.ID = getStr(resp, "contract", "id")
		if contract.ID == "" {
			return "", fmt.Errorf("no contract id in response: %v", resp)
		}
		return contract.ID, nil
	})


	r.Section("5. Payment collection")
	expected := computeFees(bidAmount, settings)
	var paymentIntentID, chargeID string
	r.MustCheck("client creates payment intent (server computes the charge)", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v3/payments/intent", client.AccessToken, map[string]any{
			"proposal_id": proposal.ID,
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		data := getMap(resp, "data")
		paymentIntentID = getStr(data, "payment_intent_id")
		if paymentIntentID == "" {
			return "", fmt.Errorf("no payment_intent_id in response: %v", resp)
		}
		amount := getFloat(data, "amount")
		if math.Abs(amount-expected.ClientTotal) > 0.01 {
			return "", fmt.Errorf("client total mismatch: server=%.2f expected=%.2f", amount, expected.ClientTotal)
		}
		return fmt.Sprintf("%s charged £%.2f (matches expected fee calc)", paymentIntentID, amount), nil
	})

	r.Check("payment summary matches independently computed fee breakdown", func() (string, error) {
		status, resp, err := api.Do("GET", "/api/proposals/"+proposal.ID, client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		summary := getMap(resp, "payment_summary")
		freelancerNet := getFloat(summary, "freelancer", "amount_to_receive")
		platformEarn := getFloat(summary, "platform_earnings")
		if math.Abs(freelancerNet-expected.FreelancerNet) > 0.01 {
			return "", fmt.Errorf("freelancer net mismatch: got=%.2f expected=%.2f", freelancerNet, expected.FreelancerNet)
		}
		if math.Abs(platformEarn-expected.PlatformEarnings) > 0.01 {
			return "", fmt.Errorf("platform earnings mismatch: got=%.2f expected=%.2f", platformEarn, expected.PlatformEarnings)
		}
		return fmt.Sprintf("freelancer receives £%.2f, platform keeps £%.2f", freelancerNet, platformEarn), nil
	})

	r.MustCheck("customer object linked to payment intent", func() (string, error) {
		pi, err := sc.Get("/payment_intents/"+paymentIntentID, "")
		if err != nil {
			return "", err
		}
		cust := getStr(pi, "customer")
		if cust == "" {
			return "", fmt.Errorf("payment intent has no linked stripe customer")
		}
		return "customer=" + cust, nil
	})

	r.MustCheck("confirm payment intent with stripe test card", func() (string, error) {
		form := url.Values{
			"payment_method": {"pm_card_visa"},
			"return_url":     {"https://example.com/return"},
		}
		pi, err := sc.Post("/payment_intents/"+paymentIntentID+"/confirm", form, "")
		if err != nil {
			return "", err
		}
		status := getStr(pi, "status")
		if status != "succeeded" {
			return "", fmt.Errorf("unexpected status: %s", status)
		}
		chargeID = getStr(pi, "latest_charge")
		return "status=succeeded charge=" + chargeID, nil
	})

	r.Check("charge.succeeded webhook recorded a held payment", func() (string, error) {
		return pollUntil(20*time.Second, func() (string, bool, error) {
			status, resp, err := api.Do("GET", "/api/v3/payments/transactions", client.AccessToken, nil, nil)
			if err != nil {
				return "", false, err
			}
			if status != 200 {
				return "", false, fmt.Errorf("status %d: %v", status, resp)
			}
			for _, txAny := range getSliceRaw(resp, "transactions") {
				tx, _ := txAny.(map[string]any)
				if tx == nil {
					continue
				}
				if getStr(tx, "contract_id") == contract.ID {
					st := getStr(tx, "status")
					if st == "held" || st == "released" {
						return fmt.Sprintf("status=%s gross=%.2f net=%.2f", st, getFloat(tx, "gross_amount"), getFloat(tx, "net_amount")), true, nil
					}
					return "status=" + st, false, nil
				}
			}
			return "no matching transaction yet", false, nil
		})
	})

	// ---------------------------------------------------------------------
	r.Section("6. Completion and auto-release")

	r.Check("first confirmation does not release payment", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/contracts/"+contract.ID+"/complete", freelancer.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		release := getStr(resp, "payment_release", "status")
		if release != "not_applicable" {
			return "", fmt.Errorf("expected not_applicable, got %s", release)
		}
		return "payment_release=not_applicable (waiting for other party)", nil
	})

	var transferID string
	r.MustCheck("second confirmation triggers auto-release", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/contracts/"+contract.ID+"/complete", client.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		release := getStr(resp, "payment_release", "status")
		if release != "released" {
			return "", fmt.Errorf("expected released, got %s (%v)", release, resp["payment_release"])
		}
		return "payment_release=released", nil
	})

	r.Check("stripe transfer exists, tied to original charge, correct amount", func() (string, error) {
		return pollUntil(15*time.Second, func() (string, bool, error) {
			list, err := sc.Get("/transfers?limit=10&transfer_group="+contract.ID, "")
			if err != nil {
				return "", false, err
			}
			for _, tAny := range getSliceRaw(list, "data") {
				t, _ := tAny.(map[string]any)
				if t == nil {
					continue
				}
				transferID = getStr(t, "id")
				amountPence := int(getFloat(t, "amount"))
				expectedPence := int(math.Round(expected.FreelancerNet * 100))
				if amountPence != expectedPence {
					return "", false, fmt.Errorf("transfer amount %dp != expected %dp", amountPence, expectedPence)
				}
				src := getStr(t, "source_transaction")
				if src != chargeID {
					return "", false, fmt.Errorf("transfer source_transaction=%s != charge=%s", src, chargeID)
				}
				dest := getStr(t, "destination")
				if dest != freelancer.StripeAccountID {
					return "", false, fmt.Errorf("transfer destination=%s != freelancer account=%s", dest, freelancer.StripeAccountID)
				}
				return fmt.Sprintf("transfer=%s amount=%dp -> %s", transferID, amountPence, dest), true, nil
			}
			return "no transfer yet", false, nil
		})
	})

	r.Check("duplicate release is refused (no double payout)", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v3/escrow/contracts/"+contract.ID+"/release", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			return "", fmt.Errorf("expected release to be refused, but got 200: %v", resp)
		}
		return fmt.Sprintf("status=%d, correctly refused: %v", status, resp["error"]), nil
	})

	r.Check("payment transaction shows released", func() (string, error) {
		status, resp, err := api.Do("GET", "/api/v3/payments/transactions", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d", status)
		}
		for _, txAny := range getSliceRaw(resp, "transactions") {
			tx, _ := txAny.(map[string]any)
			if tx != nil && getStr(tx, "contract_id") == contract.ID {
				st := getStr(tx, "status")
				if st != "released" {
					return "", fmt.Errorf("expected released, got %s", st)
				}
				return "status=released", nil
			}
		}
		return "", fmt.Errorf("transaction not found")
	})

	// ---------------------------------------------------------------------
	r.Section("7. Wallet, payout balance, withdrawal")

	r.Check("freelancer wallet reflects released payment", func() (string, error) {
		status, resp, err := api.Do("GET", "/api/v3/wallet", freelancer.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		balance := getFloat(resp, "data", "balance")
		if math.Abs(balance-expected.FreelancerNet) > 0.01 {
			return "", fmt.Errorf("wallet balance %.2f != expected %.2f", balance, expected.FreelancerNet)
		}
		return fmt.Sprintf("balance=£%.2f", balance), nil
	})

	r.Check("payout balance shows funds pending on stripe", func() (string, error) {
		status, resp, err := api.Do("GET", "/api/v3/wallet/payout-balance", freelancer.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		available := getFloat(resp, "data", "available")
		pending := getFloat(resp, "data", "pending")
		if pending <= 0 && available <= 0 {
			return "", fmt.Errorf("expected released funds to show as pending or available, got both zero")
		}
		return fmt.Sprintf("available=£%.2f pending=£%.2f", available, pending), nil
	})

	r.Warn("withdrawal (expected to be refused: stripe test mode holds funds ~5 days like production)", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v3/wallet/withdraw", freelancer.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			return fmt.Sprintf("withdrawal succeeded (balance was already available): %v", resp), nil
		}
		return "", fmt.Errorf("refused as expected: %v", resp["error"])
	})

	// ---------------------------------------------------------------------
	r.Section("8. Dispute gate blocks completion and release")

	job2 := &TestJob{}
	r.MustCheck("create second job for dispute test", func() (string, error) {
		id, err := createJob(api, client, catID, "Payment flow dispute test job", bidAmount)
		job2.ID = id
		return id, err
	})

	proposal2 := &TestProposal{}
	r.MustCheck("submit and accept second proposal", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/job/send-proposal", freelancer.AccessToken, map[string]any{
			"job_post_id": job2.ID, "cover_letter": "second test job", "bid_amount": bidAmount, "duration": 1,
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		proposal2.ID = getStr(resp, "data", "id")

		status, resp, err = api.Do("POST", "/api/proposals/"+proposal2.ID+"/decision", client.AccessToken, map[string]any{"status": "accepted"}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("accept status %d: %v", status, resp)
		}
		return proposal2.ID, nil
	})

	contract2 := &TestContract{}
	r.MustCheck("create second contract and pay", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/proposals/create-contract", client.AccessToken, map[string]any{
			"proposal_id": proposal2.ID, "title": "Dispute test job", "description": "created by paymenttest",
			"total_amount": bidAmount,
			"start_date":   time.Now().Format(time.RFC3339),
			"end_date":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"terms":        "Standard",
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("contract status %d: %v", status, resp)
		}
		contract2.ID = getStr(resp, "contract", "id")

		status, resp, err = api.Do("POST", "/api/v3/payments/intent", client.AccessToken, map[string]any{"proposal_id": proposal2.ID}, nil)
		if err != nil {
			return "", err
		}
		pi2ID := getStr(resp, "data", "payment_intent_id")

		form := url.Values{"payment_method": {"pm_card_visa"}, "return_url": {"https://example.com/return"}}
		pi, err := sc.Post("/payment_intents/"+pi2ID+"/confirm", form, "")
		if err != nil {
			return "", err
		}
		if getStr(pi, "status") != "succeeded" {
			return "", fmt.Errorf("payment did not succeed: %v", pi)
		}
		return contract2.ID, nil
	})

	// The charge.succeeded webhook is async — wait for the escrow to actually
	// exist before filing a dispute against it, or completion later will find
	// nothing held to block/release.
	r.MustCheck("second contract: charge.succeeded webhook recorded a held payment", func() (string, error) {
		return pollUntil(20*time.Second, func() (string, bool, error) {
			status, resp, err := api.Do("GET", "/api/v3/payments/transactions", client.AccessToken, nil, nil)
			if err != nil {
				return "", false, err
			}
			if status != 200 {
				return "", false, fmt.Errorf("status %d: %v", status, resp)
			}
			for _, txAny := range getSliceRaw(resp, "transactions") {
				tx, _ := txAny.(map[string]any)
				if tx != nil && getStr(tx, "contract_id") == contract2.ID {
					st := getStr(tx, "status")
					if st == "held" || st == "released" {
						return "status=" + st, true, nil
					}
					return "status=" + st, false, nil
				}
			}
			return "no matching transaction yet", false, nil
		})
	})

	disputeID := ""
	r.MustCheck("freelancer files a dispute", func() (string, error) {
		form := url.Values{
			"contract_id": {contract2.ID},
			"reason":      {"Client unresponsive"},
			"description": {"Client stopped responding after job was posted (paymenttest)"},
		}
		status, resp, err := api.PostForm("/api/v1/disputes", freelancer.AccessToken, form, nil)
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		disputeID = getStr(resp, "data", "id")
		if disputeID == "" {
			return "", fmt.Errorf("no dispute id: %v", resp)
		}
		return disputeID, nil
	})

	r.Check("completion is blocked while dispute is open", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/contracts/"+contract2.ID+"/complete", client.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			return "", fmt.Errorf("expected completion to be blocked, got 200: %v", resp)
		}
		return fmt.Sprintf("blocked (status %d): %v", status, resp["error"]), nil
	})

	r.Check("manual release is blocked while dispute is open", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/v3/escrow/contracts/"+contract2.ID+"/release", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			return "", fmt.Errorf("expected release to be blocked, got 200: %v", resp)
		}
		return fmt.Sprintf("blocked (status %d): %v", status, resp["error"]), nil
	})

	// This freshly-registered client has no admin role (granting one requires
	// direct DB access, which this tool deliberately avoids — see section 8
	// for the corresponding "non-admin is refused" check). What we *can*
	// verify without admin access: the payment stays held, not released,
	// through every blocked attempt above.
	r.Check("payment still held (not released) after blocked attempts", func() (string, error) {
		status, resp, err := api.Do("GET", "/api/v3/payments/transactions", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		for _, txAny := range getSliceRaw(resp, "transactions") {
			tx, _ := txAny.(map[string]any)
			if tx != nil && getStr(tx, "contract_id") == contract2.ID {
				st := getStr(tx, "status")
				if st != "held" {
					return "", fmt.Errorf("expected still held, got %s", st)
				}
				return "status=held", nil
			}
		}
		return "", fmt.Errorf("transaction not found")
	})

	if *adminToken == "" {
		fmt.Printf("%s[SKIP]%s %-55s %s\n", colorYellow, colorReset,
			"admin force-release is ALSO blocked while dispute is open",
			"no -admin-token supplied; grant one user the 'admin' role and pass its access token to test this")
	} else {
		r.Check("admin force-release is ALSO blocked while dispute is open", func() (string, error) {
			status, resp, err := api.Do("GET", "/api/v3/admin/payouts/escrows?status=held", *adminToken, nil, nil)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("status %d: %v", status, resp)
			}
			escrowID2 := ""
			for _, rowAny := range getSliceRaw(resp, "data") {
				row, _ := rowAny.(map[string]any)
				if row != nil && getStr(row, "contract", "id") == contract2.ID {
					escrowID2 = getStr(row, "id")
				}
			}
			if escrowID2 == "" {
				return "", fmt.Errorf("held escrow for disputed contract not found in admin list")
			}
			status, resp, err = api.Do("POST", "/api/v3/admin/payouts/escrows/"+escrowID2+"/release", *adminToken, map[string]any{"note": "paymenttest override attempt"}, nil)
			if err != nil {
				return "", err
			}
			if status == 200 {
				return "", fmt.Errorf("expected admin release to be blocked, got 200: %v", resp)
			}
			return fmt.Sprintf("blocked (status %d): %v", status, resp["error"]), nil
		})
	}

	r.MustCheck("admin resolves the dispute", func() (string, error) {
		status, resp, err := api.Do("PUT", "/api/v1/admin/disputes/"+disputeID+"/resolve", client.AccessToken, map[string]any{
			"resolution": "paymenttest: resolved automatically",
		}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		return "resolved, contract -> active", nil
	})

	r.MustCheck("completion + release now works after dispute resolution", func() (string, error) {
		status, resp, err := api.Do("POST", "/api/contracts/"+contract2.ID+"/complete", freelancer.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		status, resp, err = api.Do("POST", "/api/contracts/"+contract2.ID+"/complete", client.AccessToken, map[string]any{}, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %v", status, resp)
		}
		release := getStr(resp, "payment_release", "status")
		if release != "released" {
			return "", fmt.Errorf("expected released, got %s", release)
		}
		return "payment_release=released", nil
	})

	// ---------------------------------------------------------------------
	r.Section("9. Admin visibility")

	r.Check("non-admin is blocked from admin payout routes", func() (string, error) {
		status, _, err := api.Do("GET", "/api/v3/admin/payouts/escrows", freelancer.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if status != 403 {
			return "", fmt.Errorf("expected 403 for non-admin, got %d", status)
		}
		return "403 as expected", nil
	})

	if *adminToken == "" {
		fmt.Printf("%s[SKIP]%s %-55s %s\n", colorYellow, colorReset,
			"admin escrow list shows both released escrows",
			"no -admin-token supplied; grant one user the 'admin' role and pass its access token to test this")
	} else {
		r.Check("admin escrow list shows both released escrows", func() (string, error) {
			status, resp, err := api.Do("GET", "/api/v3/admin/payouts/escrows?status=released", *adminToken, nil, nil)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("status %d: %v", status, resp)
			}
			found := 0
			for _, rowAny := range getSliceRaw(resp, "data") {
				row, _ := rowAny.(map[string]any)
				if row == nil {
					continue
				}
				cid := getStr(row, "contract", "id")
				if cid == contract.ID || cid == contract2.ID {
					found++
				}
			}
			if found != 2 {
				return "", fmt.Errorf("expected both test contracts in admin list, found %d", found)
			}
			return "both contracts visible to admin", nil
		})
	}

	r.Summary()
	fmt.Println("\ntest data created this run (not cleaned up):")
	fmt.Printf("  client:     %s / %s (stripe: %s)\n", client.Email, client.ID, client.StripeAccountID)
	fmt.Printf("  freelancer: %s / %s (stripe: %s)\n", freelancer.Email, freelancer.ID, freelancer.StripeAccountID)
	fmt.Printf("  jobs: %s, %s\n", job.ID, job2.ID)
	fmt.Printf("  contracts: %s, %s\n", contract.ID, contract2.ID)

	os.Exit(r.ExitCode())
}

// --- domain helpers -------------------------------------------------------

type TestUser struct {
	Email           string
	Password        string
	ID              string
	AccessToken     string
	StripeAccountID string
}

type TestJob struct{ ID string }
type TestProposal struct{ ID string }
type TestContract struct{ ID string }

type SystemSettings struct {
	ClientCommissionPct     float64
	FreelancerCommissionPct float64
	PlatformFee             float64
}

func parseSettings(m map[string]any) SystemSettings {
	return SystemSettings{
		ClientCommissionPct:     getFloat(m, "client_commission_percentage"),
		FreelancerCommissionPct: getFloat(m, "freelancer_commission_percentage"),
		PlatformFee:             getFloat(m, "application_fee_amount"),
	}
}

type ExpectedFees struct {
	ClientTotal      float64
	FreelancerNet    float64
	PlatformEarnings float64
}

// computeFees replicates the server's fee formula independently, so this tool
// verifies the server's math rather than just asserting whatever it returns.
func computeFees(bid float64, s SystemSettings) ExpectedFees {
	round2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	clientCommission := round2(bid * s.ClientCommissionPct / 100)
	freelancerCommission := round2(bid * s.FreelancerCommissionPct / 100)
	platformFee := round2(s.PlatformFee)
	clientTotal := round2(bid + clientCommission + platformFee)
	freelancerNet := round2(bid - freelancerCommission - platformFee)
	return ExpectedFees{
		ClientTotal:      clientTotal,
		FreelancerNet:    freelancerNet,
		PlatformEarnings: round2(clientTotal - freelancerNet),
	}
}

// registerUser posts as urlencoded form data, matching /api/auth/register's
// `form:` binding (UserData). dob is deliberately omitted: the endpoint has no
// time_format tag on that field and rejects a plain date.
func registerUser(api *APIClient, u *TestUser, lastName string) (string, error) {
	form := url.Values{
		"first_name":   {"PayTest"},
		"last_name":    {lastName},
		"email":        {u.Email},
		"password":     {u.Password},
		"phone_number": {randomUKPhone()},
	}
	status, resp, err := api.PostForm("/api/auth/register", "", form, nil)
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("status %d: %v", status, resp)
	}
	u.AccessToken = getStr(resp, "tokens", "access_token")
	u.ID = getStr(resp, "user", "id")
	u.StripeAccountID = getStr(resp, "user", "stripe_connect_account_id")
	if u.AccessToken == "" || u.ID == "" {
		return "", fmt.Errorf("missing token/id in response: %v", resp)
	}

	// Registration only fires Stripe provisioning async; poll briefly for the
	// connect account id to show up before moving on.
	for i := 0; i < 10 && u.StripeAccountID == ""; i++ {
		time.Sleep(1 * time.Second)
		_, loginResp, _ := api.Do("POST", "/api/auth/login", "", map[string]any{"email": u.Email, "password": u.Password}, nil)
		if id := getStr(loginResp, "user", "stripe_connect_account_id"); id != "" {
			u.StripeAccountID = id
		}
	}

	return fmt.Sprintf("id=%s email=%s", u.ID, u.Email), nil
}

func loginUser(api *APIClient, u *TestUser) (string, error) {
	status, resp, err := api.Do("POST", "/api/auth/login", "", map[string]any{
		"email": u.Email, "password": u.Password,
	}, nil)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("status %d: %v", status, resp)
	}
	u.AccessToken = getStr(resp, "tokens", "access_token")
	if id := getStr(resp, "user", "stripe_connect_account_id"); id != "" {
		u.StripeAccountID = id
	}
	return "logged in", nil
}

// createJob posts as urlencoded form data, matching /api/job/create's `form:` binding.
func createJob(api *APIClient, owner *TestUser, catID, title string, budget float64) (string, error) {
	form := url.Values{
		"category_id": {catID},
		"title":       {title},
		"description": {"Created by the paymenttest tool to validate the payment flow."},
		"budget":      {strconv.FormatFloat(budget, 'f', 2, 64)},
		"open_budget": {"false"},
	}
	status, resp, err := api.PostForm("/api/job/create", owner.AccessToken, form, nil)
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("status %d: %v", status, resp)
	}
	id := getStr(resp, "data", "id")
	if id == "" {
		return "", fmt.Errorf("no job id in response: %v", resp)
	}
	return id, nil
}

func randomUKPhone() string {
	return fmt.Sprintf("+4477%08d", rand.Intn(100000000))
}

func filterOut(items []string, exclude string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it != exclude {
			out = append(out, it)
		}
	}
	return out
}

func pollUntil(timeout time.Duration, fn func() (string, bool, error)) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastDetail string
	var lastErr error
	for time.Now().Before(deadline) {
		detail, done, err := fn()
		if err != nil {
			return "", err
		}
		if done {
			return detail, nil
		}
		lastDetail = detail
		time.Sleep(1500 * time.Millisecond)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("timed out after %s (last: %s)", timeout, lastDetail)
}
