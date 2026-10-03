// paymenttest drives the whole Tasksy payment flow against a running server
// and real Stripe test mode: onboarding, jobs, proposals, contracts, payment,
// escrow, release, wallet, withdrawal, disputes and refunds — every step, its
// edge cases, and its failure paths.
//
//	STRIPE_SECRET_KEY=sk_test_... go run ./cmd/paymenttest \
//	    [-base-url http://localhost:8080] [-db "$DB_URI"] [-admin-token ...]
//
// Requires webhooks forwarded to the server, including Connect events, e.g.
//
//	stripe listen --forward-to localhost:8080/api/v3/webhooks/stripe/payment \
//	    --forward-connect-to localhost:8080/api/v3/webhooks/stripe/payment
//
// -db (a Postgres DSN) enables DB invariant checks and auto-provisions an
// admin user; without it, pass -admin-token or admin-only checks are skipped.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "Tasksy API base URL")
	stripeKey := flag.String("stripe-key", os.Getenv("STRIPE_SECRET_KEY"), "Stripe secret key (test mode only)")
	adminToken := flag.String("admin-token", os.Getenv("ADMIN_ACCESS_TOKEN"), "access token of a user with the admin role")
	dsn := flag.String("db", os.Getenv("PAYMENTTEST_DB"), "Postgres DSN of the server's database (local/staging only)")
	flag.Parse()

	if !strings.HasPrefix(*stripeKey, "sk_test_") {
		fmt.Println("refusing to run: a Stripe test-mode secret key (sk_test_...) is required via STRIPE_SECRET_KEY or -stripe-key")
		os.Exit(2)
	}

	run := time.Now().Unix()
	rand.Seed(run)
	s := &Suite{api: NewAPIClient(*baseURL), sc: NewStripeClient(*stripeKey), r: &Runner{}, run: run, adminToken: *adminToken}

	fmt.Printf("%sTasksy payment flow suite%s base=%s run=%d\n", colorBold, colorReset, *baseURL, run)

	if *dsn != "" {
		db, err := openDB(*dsn)
		if err != nil {
			fmt.Printf("cannot open -db: %v\n", err)
			os.Exit(2)
		}
		s.db = db
	}

	client := newUser("client", run)
	f1 := newUser("freelancer", run)
	f2 := newUser("freelancer2", run) // stays un-onboarded until the unpayable-freelancer scenario

	s.preflight()
	s.setupUsers(client, f1, f2)
	s.unauthenticatedAccess(f1)

	s.r.Section("Stripe Connect onboarding — client")
	s.onboard(client, "Test", "Client", "")
	s.r.Section("Stripe Connect onboarding — freelancer")
	s.onboard(f1, "Test", "Freelancer", client.Phone)

	s.bankAccounts(f1, client)
	s.proposalAndContractRules(client, f1, f2)

	a := s.mainFlow(client, f1, f2)
	d := s.unpayableFreelancerFlow(client, f2)
	c := s.disputeFlow(client, f1, a)
	e := s.refundHeldFlow(client, f1)
	s.refundReleasedFlows(client, f1, c, a)
	s.webhookForgery(client)
	s.finalInvariants(client, f1, f2, []*Flow{a, c, d, e})

	s.r.Summary()
	fmt.Println("\ntest data from this run (not cleaned up):")
	for _, u := range []*TestUser{client, f1, f2} {
		fmt.Printf("  %-12s %s  id=%s  stripe=%s\n", u.Label, u.Email, u.ID, u.StripeAccountID)
	}
	for _, f := range []*Flow{a, c, d, e} {
		fmt.Printf("  flow %-10s contract=%s  payment_intent=%s\n", f.Label, f.ContractID, f.PaymentIntentID)
	}
	os.Exit(s.r.ExitCode())
}
