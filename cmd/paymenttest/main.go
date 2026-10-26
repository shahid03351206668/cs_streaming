// Usage: STRIPE_SECRET_KEY=sk_test_... go run ./cmd/paymenttest [-db DSN] [-webhook-secret whsec_...] [-admin-token ...]. Needs 'stripe listen' forwarding webhooks (Connect events included) to the server; -db enables DB checks and creates the admin user.
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
	whsec := flag.String("webhook-secret", os.Getenv("PAYMENTTEST_WEBHOOK_SECRET"), "the server's Stripe webhook signing secret (enables signed webhook replay checks)")
	flag.Parse()

	if !strings.HasPrefix(*stripeKey, "sk_test_") {
		fmt.Println("refusing to run: a Stripe test-mode secret key (sk_test_...) is required via STRIPE_SECRET_KEY or -stripe-key")
		os.Exit(2)
	}

	run := time.Now().Unix()
	rand.Seed(run)
	s := &Suite{api: NewAPIClient(*baseURL), sc: NewStripeClient(*stripeKey), r: &Runner{}, run: run, adminToken: *adminToken, webhookSecret: *whsec}

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

	if s.db == nil && s.adminToken == "" {
		fmt.Println("an admin is required: pass -db (creates one) or -admin-token")
		os.Exit(2)
	}

	s.preflight()
	s.setupUsers(client, f1, f2)
	s.unauthenticatedAccess(f1)
	s.adminLoginSecurity(client)

	s.r.Section("Stripe Connect onboarding — client")
	s.onboard(client, "Test", "Client", "")
	s.r.Section("Stripe Connect onboarding — freelancer")
	s.onboard(f1, "Test", "Freelancer", client.Phone)

	s.bankAccounts(f1, client)
	s.proposalAndContractRules(client, f1, f2)

	a := s.mainFlow(client, f1, f2)
	d := s.unpayableFreelancerFlow(client, f2)
	pf := s.payBeforeContractFlow(client, f2)
	c := s.disputeFlow(client, f1, a)
	e := s.refundHeldFlow(client, f1)
	s.refundReleasedFlows(client, f1, c, a)

	g := s.raceFlow(client, f2)
	h := s.concurrentRefundFlow(client, f2)
	ij := s.dashboardRefundFlows(client, f2)
	k := s.chargebackFlow(client, f2)
	l := s.payBeforeAcceptFlows(client, f2)
	dispFlows := s.disputeOutcomeFlows(client, f2)
	s.reviewFlows(client, f1, f2, a, d, pf)
	s.webhookReplays(map[string]*Flow{"A": a, "C": c, "E": e, "F": pf, "G": g})
	s.webhookForgery(client)

	flows := append([]*Flow{a, c, d, e, pf, g, h, ij[0], ij[1], k, l}, dispFlows...)
	s.finalInvariants(client, f1, f2, flows)

	s.r.Summary()
	fmt.Println("\ntest data from this run (not cleaned up):")
	for _, u := range []*TestUser{client, f1, f2} {
		fmt.Printf("  %-12s %s  id=%s  stripe=%s\n", u.Label, u.Email, u.ID, u.StripeAccountID)
	}
	for _, f := range flows {
		fmt.Printf("  flow %-10s contract=%s  payment_intent=%s\n", f.Label, f.ContractID, f.PaymentIntentID)
	}
	os.Exit(s.r.ExitCode())
}
