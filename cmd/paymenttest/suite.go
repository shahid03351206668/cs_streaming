package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Stripe test payment methods.
const (
	pmVisa          = "pm_card_visa"
	pmBypassPending = "pm_card_bypassPending" // funds land in the available balance immediately
	pmDeclined      = "pm_card_chargeDeclined"
)

type Suite struct {
	api        *APIClient
	sc         *StripeClient
	db         *DB // nil when -db isn't given; DB-level checks are skipped
	r          *Runner
	run        int64
	settings   SystemSettings
	admin      *TestUser
	adminToken string
	catID      string

	inProgressJobID string // a job whose proposal was accepted; must refuse new proposals
}

type TestUser struct {
	Label           string
	Email           string
	Password        string
	ID              string
	AccessToken     string
	StripeAccountID string
	Phone           string // phone submitted to Stripe via the requirements endpoint
	BankID          string // local id of the default bank account
	BankStripeID    string
	Ledger          *Ledger
}

// Flow is one job -> proposal -> contract -> payment.
type Flow struct {
	Label           string
	Client          *TestUser
	Freelancer      *TestUser
	Fees            ExpectedFees
	JobID           string
	ProposalID      string
	ContractID      string
	PaymentIntentID string
	ChargeID        string
	TransactionID   string
	EscrowID        string
	TransferID      string
	FinalStatus     string // expected escrow/payment status at the end of the run
}

func newUser(label string, run int64) *TestUser {
	return &TestUser{
		Label:    label,
		Email:    fmt.Sprintf("paytest-%s-%d@example.com", label, run),
		Password: "Password123",
		Ledger:   newLedger(),
	}
}

func randomUKPhone() string {
	return fmt.Sprintf("+4477%08d", rand.Intn(100000000))
}

// expect turns a status/response pair into an error unless the status is one of want.
func expect(status int, resp map[string]any, want ...int) error {
	for _, w := range want {
		if status == w {
			return nil
		}
	}
	return fmt.Errorf("status %d (want %v): %s", status, want, errMsg(resp))
}

// expectRefused passes for any 4xx and fails for 2xx/5xx — a refusal must be
// a deliberate client error, never a crash or a silent success.
func expectRefused(status int, resp map[string]any, mustContain string) (string, error) {
	if status < 400 || status >= 500 {
		return "", fmt.Errorf("expected a 4xx refusal, got %d: %s", status, brief(resp))
	}
	msg := errMsg(resp)
	if mustContain != "" && !strings.Contains(strings.ToLower(msg), strings.ToLower(mustContain)) {
		return "", fmt.Errorf("refused with %d but message %q does not mention %q", status, msg, mustContain)
	}
	return fmt.Sprintf("%d: %s", status, msg), nil
}

func pollUntil(timeout time.Duration, fn func() (string, bool, error)) (string, error) {
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		detail, done, err := fn()
		if err != nil {
			return "", err
		}
		if done {
			return detail, nil
		}
		last = detail
		time.Sleep(1500 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out after %s (last: %s)", timeout, last)
}

// --- users ----------------------------------------------------------------

// register posts urlencoded form data, matching /api/auth/register's `form:` binding.
func (s *Suite) register(u *TestUser, first, last string) (string, error) {
	form := url.Values{
		"first_name":   {first},
		"last_name":    {last},
		"email":        {u.Email},
		"password":     {u.Password},
		"phone_number": {randomUKPhone()},
	}
	status, resp, err := s.api.PostForm("/api/auth/register", "", form, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200, 201); err != nil {
		return "", err
	}
	u.AccessToken = getStr(resp, "tokens", "access_token")
	u.ID = getStr(resp, "user", "id")
	u.StripeAccountID = getStr(resp, "user", "stripe_connect_account_id")
	if u.AccessToken == "" || u.ID == "" {
		return "", fmt.Errorf("missing token/id in response: %v", resp)
	}
	// The Connect account is provisioned in a goroutine after registration.
	for i := 0; i < 15 && u.StripeAccountID == ""; i++ {
		time.Sleep(time.Second)
		_, _ = s.login(u)
	}
	if u.StripeAccountID == "" {
		return "", fmt.Errorf("no stripe_connect_account_id after 15s")
	}
	return fmt.Sprintf("id=%s stripe=%s", u.ID, u.StripeAccountID), nil
}

func (s *Suite) login(u *TestUser) (string, error) {
	status, resp, err := s.api.Do("POST", "/api/auth/login", "", map[string]any{"email": u.Email, "password": u.Password}, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200); err != nil {
		return "", err
	}
	u.AccessToken = getStr(resp, "tokens", "access_token")
	if u.ID == "" {
		u.ID = getStr(resp, "user", "id")
	}
	if id := getStr(resp, "user", "stripe_connect_account_id"); id != "" {
		u.StripeAccountID = id
	}
	return "logged in", nil
}

func (s *Suite) verifyIdentity(u *TestUser) (string, error) {
	status, resp, err := s.api.Do("PUT", "/api/v1/admin/users/"+u.ID, s.adminToken, map[string]any{"identity_verified": true}, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200); err != nil {
		return "", err
	}
	_, err = s.login(u) // the auth middleware loads the user per request, but refresh anyway
	return "identity_verified=true", err
}

// --- jobs, proposals, contracts --------------------------------------------

// createJob posts urlencoded form data, matching /api/job/create's `form:` binding.
func (s *Suite) createJob(owner *TestUser, title string, budget float64) (string, error) {
	form := url.Values{
		"category_id": {s.catID},
		"title":       {title},
		"description": {"Created by the paymenttest suite to validate the payment flow."},
		"budget":      {strconv.FormatFloat(budget, 'f', 2, 64)},
		"open_budget": {"false"},
	}
	status, resp, err := s.api.PostForm("/api/job/create", owner.AccessToken, form, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200, 201); err != nil {
		return "", err
	}
	id := getStr(resp, "data", "id")
	if id == "" {
		return "", fmt.Errorf("no job id in response: %v", resp)
	}
	return id, nil
}

func (s *Suite) sendProposal(freelancer *TestUser, jobID string, bid float64, extra map[string]any) (int, map[string]any, error) {
	body := map[string]any{
		"job_post_id":  jobID,
		"cover_letter": "I will complete this job well and on time.",
		"bid_amount":   bid,
		"duration":     1,
	}
	for k, v := range extra {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return s.api.Do("POST", "/api/job/send-proposal", freelancer.AccessToken, body, nil)
}

func (s *Suite) decide(token, proposalID, decision string) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/proposals/"+proposalID+"/decision", token, map[string]any{"status": decision}, nil)
}

func (s *Suite) createContract(token, proposalID string, amount float64, start, end time.Time) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/proposals/create-contract", token, map[string]any{
		"proposal_id":  proposalID,
		"title":        "paymenttest contract",
		"description":  "created by paymenttest",
		"total_amount": amount,
		"start_date":   start.Format(time.RFC3339),
		"end_date":     end.Format(time.RFC3339),
		"terms":        "Standard",
	}, nil)
}

// setupFlow creates job -> proposal -> acceptance -> contract, aborting on failure.
func (s *Suite) setupFlow(label string, client, freelancer *TestUser, bid float64) *Flow {
	f := &Flow{Label: label, Client: client, Freelancer: freelancer, Fees: computeFees(bid, s.settings)}
	s.r.MustCheck(label+": client posts job", func() (string, error) {
		id, err := s.createJob(client, "paytest "+label, bid)
		f.JobID = id
		return id, err
	})
	s.r.MustCheck(label+": freelancer sends proposal", func() (string, error) {
		status, resp, err := s.sendProposal(freelancer, f.JobID, bid, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		f.ProposalID = getStr(resp, "data", "id")
		return f.ProposalID, nil
	})
	s.r.MustCheck(label+": client accepts proposal", func() (string, error) {
		status, resp, err := s.decide(client.AccessToken, f.ProposalID, "accepted")
		if err != nil {
			return "", err
		}
		return getStr(resp, "status"), expect(status, resp, 200)
	})
	s.r.MustCheck(label+": client creates contract", func() (string, error) {
		status, resp, err := s.createContract(client.AccessToken, f.ProposalID, bid, time.Now(), time.Now().Add(24*time.Hour))
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		f.ContractID = getStr(resp, "contract", "id")
		return f.ContractID, nil
	})
	return f
}

// --- payments ---------------------------------------------------------------

func (s *Suite) createIntent(token, proposalID string, extra map[string]any) (int, map[string]any, error) {
	body := map[string]any{"proposal_id": proposalID}
	for k, v := range extra {
		body[k] = v
	}
	return s.api.Do("POST", "/api/v3/payments/intent", token, body, nil)
}

func (s *Suite) confirmIntent(piID, pm string) (map[string]any, error) {
	return s.sc.Post("/payment_intents/"+piID+"/confirm", url.Values{
		"payment_method": {pm},
		"return_url":     {"https://example.com/return"},
	}, "")
}

// contractTransactions returns every payment row the client sees for a contract.
func (s *Suite) contractTransactions(token, contractID string) ([]map[string]any, map[string]any, error) {
	status, resp, err := s.api.Do("GET", "/api/v3/payments/transactions", token, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := expect(status, resp, 200); err != nil {
		return nil, nil, err
	}
	var rows []map[string]any
	for _, row := range getRows(resp, "transactions") {
		if getStr(row, "contract_id") == contractID && getStr(row, "direction") == "paid" {
			rows = append(rows, row)
		}
	}
	return rows, resp, nil
}

func (s *Suite) waitForStatus(token, contractID, want string, timeout time.Duration) (map[string]any, error) {
	var found map[string]any
	_, err := pollUntil(timeout, func() (string, bool, error) {
		rows, _, err := s.contractTransactions(token, contractID)
		if err != nil {
			return "", false, err
		}
		if len(rows) == 0 {
			return "no transaction yet", false, nil
		}
		found = rows[0]
		st := getStr(found, "status")
		return "status=" + st, st == want, nil
	})
	return found, err
}

// payFlow creates the intent and confirms it with pm, then waits for the
// charge.succeeded webhook to record exactly one held payment.
func (s *Suite) payFlow(f *Flow, pm string) {
	s.r.MustCheck(f.Label+": client creates payment intent", func() (string, error) {
		status, resp, err := s.createIntent(f.Client.AccessToken, f.ProposalID, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		f.PaymentIntentID = getStr(resp, "data", "payment_intent_id")
		if amt := getNum(resp, "data", "amount"); !eq(amt, f.Fees.ClientTotal) {
			return "", fmt.Errorf("charged £%.2f, expected £%.2f", amt, f.Fees.ClientTotal)
		}
		return fmt.Sprintf("%s £%.2f", f.PaymentIntentID, f.Fees.ClientTotal), nil
	})
	s.r.MustCheck(f.Label+": confirm payment ("+pm+")", func() (string, error) {
		pi, err := s.confirmIntent(f.PaymentIntentID, pm)
		if err != nil {
			return "", err
		}
		if st := getStr(pi, "status"); st != "succeeded" {
			return "", fmt.Errorf("payment intent status %s", st)
		}
		f.ChargeID = getStr(pi, "latest_charge")
		return "charge=" + f.ChargeID, nil
	})
	s.r.MustCheck(f.Label+": webhook records held payment", func() (string, error) {
		tx, err := s.waitForStatus(f.Client.AccessToken, f.ContractID, "held", 30*time.Second)
		if err != nil {
			return "", err
		}
		f.TransactionID = getStr(tx, "id")
		return "transaction=" + f.TransactionID, nil
	})
	f.Freelancer.Ledger.hold(f.ContractID, f.Fees.FreelancerNet)
	f.Client.Ledger.pay(f.TransactionID, f.Fees.ClientTotal)
	s.checkPaymentRow(f, "held")
}

func (s *Suite) complete(u *TestUser, contractID string) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/contracts/"+contractID+"/complete", u.AccessToken, map[string]any{}, nil)
}

func (s *Suite) releaseContract(u *TestUser, contractID string) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/v3/escrow/contracts/"+contractID+"/release", u.AccessToken, nil, nil)
}

// --- stripe verification ------------------------------------------------------

// verifyTransfer checks the Stripe transfer for a released flow end to end.
func (s *Suite) verifyTransfer(f *Flow) {
	s.r.Check(f.Label+": stripe transfer matches escrow exactly", func() (string, error) {
		list, err := s.sc.Get("/transfers?limit=10&transfer_group="+f.ContractID, "")
		if err != nil {
			return "", err
		}
		transfers := getRows(list, "data")
		if len(transfers) != 1 {
			return "", fmt.Errorf("expected exactly 1 transfer for the contract, found %d", len(transfers))
		}
		t := transfers[0]
		f.TransferID = getStr(t, "id")
		if got, want := int64(getNum(t, "amount")), pence(f.Fees.FreelancerNet); got != want {
			return "", fmt.Errorf("transfer amount %dp, expected %dp", got, want)
		}
		if src := getStr(t, "source_transaction"); src != f.ChargeID {
			return "", fmt.Errorf("source_transaction=%s, expected charge %s", src, f.ChargeID)
		}
		if dest := getStr(t, "destination"); dest != f.Freelancer.StripeAccountID {
			return "", fmt.Errorf("destination=%s, expected %s", dest, f.Freelancer.StripeAccountID)
		}
		if f.EscrowID != "" && getStr(t, "metadata", "escrow_id") != f.EscrowID {
			return "", fmt.Errorf("metadata.escrow_id=%s, expected %s", getStr(t, "metadata", "escrow_id"), f.EscrowID)
		}
		return fmt.Sprintf("%s %dp -> %s from %s", f.TransferID, pence(f.Fees.FreelancerNet), f.Freelancer.StripeAccountID, f.ChargeID), nil
	})
}

func (s *Suite) noTransfer(f *Flow) {
	s.r.Check(f.Label+": no stripe transfer was made", func() (string, error) {
		list, err := s.sc.Get("/transfers?limit=10&transfer_group="+f.ContractID, "")
		if err != nil {
			return "", err
		}
		if n := len(getRows(list, "data")); n != 0 {
			return "", fmt.Errorf("found %d transfer(s) for a contract that must not pay out", n)
		}
		return "0 transfers", nil
	})
}

// --- DB invariants --------------------------------------------------------------

// checkPaymentRow asserts the stored payment + escrow for a flow: exactly one
// of each, amounts exactly as computed, both in the expected status.
func (s *Suite) checkPaymentRow(f *Flow, wantStatus string) {
	name := f.Label + ": DB has exactly one payment+escrow, status " + wantStatus
	if s.db == nil {
		s.r.Skip(name, "no -db")
		return
	}
	s.r.Check(name, func() (string, error) {
		pays, err := s.db.payments(f.ContractID)
		if err != nil {
			return "", err
		}
		escs, err := s.db.escrows(f.ContractID)
		if err != nil {
			return "", err
		}
		if len(pays) != 1 || len(escs) != 1 {
			return "", fmt.Errorf("found %d payment rows and %d escrow rows (want 1 and 1) — duplicate charge?", len(pays), len(escs))
		}
		p, e := pays[0], escs[0]
		f.EscrowID = e.ID
		var problems []string
		if p.Status != wantStatus {
			problems = append(problems, "payment.status="+p.Status)
		}
		if e.Status != wantStatus {
			problems = append(problems, "escrow.status="+e.Status)
		}
		if e.PaymentTransactionID != p.ID {
			problems = append(problems, "escrow not linked to payment")
		}
		fees := f.Fees
		for _, c := range []struct {
			field     string
			got, want float64
		}{
			{"gross_amount", p.GrossAmount, fees.ClientTotal},
			{"net_amount", p.NetAmount, fees.FreelancerNet},
			{"platform_fee", p.PlatformFee, fees.PlatformEarnings},
			{"client_fee", p.ClientFee, fees.ClientFee},
			{"freelancer_fee", p.FreelancerFee, fees.FreelancerFee},
			{"escrow.amount", e.Amount, fees.FreelancerNet},
		} {
			if !eq(c.got, c.want) {
				problems = append(problems, fmt.Sprintf("%s=%.2f want %.2f", c.field, c.got, c.want))
			}
		}
		if p.StripePaymentIntentID != f.PaymentIntentID {
			problems = append(problems, "payment_intent mismatch")
		}
		if wantStatus == "released" && e.StripeTransferID == "" {
			problems = append(problems, "released escrow has no stripe_transfer_id")
		}
		if wantStatus == "held" && e.StripeTransferID != "" {
			problems = append(problems, "held escrow already has a transfer id")
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return fmt.Sprintf("gross=%.2f net=%.2f platform=%.2f escrow=%s", p.GrossAmount, p.NetAmount, p.PlatformFee, e.ID), nil
	})
}

// --- wallet model ----------------------------------------------------------------

// Ledger is the suite's own model of what a user's wallet must show.
type Ledger struct {
	held        map[string]float64 // contract -> freelancer net, still in escrow
	released    map[string]float64 // contract -> freelancer net, transferred
	withdrawals []float64
	paid        map[string]float64 // transaction -> client total (client side)
	refunded    map[string]bool    // transaction -> refunded (client side)
}

func newLedger() *Ledger {
	return &Ledger{held: map[string]float64{}, released: map[string]float64{}, paid: map[string]float64{}, refunded: map[string]bool{}}
}

func (l *Ledger) hold(contract string, net float64) { l.held[contract] = net }
func (l *Ledger) release(contract string) {
	l.released[contract] = l.held[contract]
	delete(l.held, contract)
}
func (l *Ledger) dropHeld(contract string)     { delete(l.held, contract) }
func (l *Ledger) dropReleased(contract string) { delete(l.released, contract) }
func (l *Ledger) withdraw(amount float64)      { l.withdrawals = append(l.withdrawals, amount) }
func (l *Ledger) pay(tx string, total float64) { l.paid[tx] = total }
func (l *Ledger) refund(tx string)             { l.refunded[tx] = true }

func sum(m map[string]float64) float64 {
	t := 0.0
	for _, v := range m {
		t += v
	}
	return round2(t)
}

func (l *Ledger) withdrawn() float64 {
	t := 0.0
	for _, v := range l.withdrawals {
		t += v
	}
	return round2(t)
}

func (l *Ledger) balance() float64 { return round2(sum(l.released) - l.withdrawn()) }

func sortedValues(m map[string]float64) []float64 {
	out := make([]float64, 0, len(m))
	for _, v := range m {
		out = append(out, round2(v))
	}
	sort.Float64s(out)
	return out
}

func sameAmounts(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	sort.Float64s(got)
	sort.Float64s(want)
	for i := range got {
		if !eq(got[i], want[i]) {
			return false
		}
	}
	return true
}

// checkWallet compares GET /api/v3/wallet against the ledger: totals, every
// transaction line, no duplicate lines, and the live Stripe balance.
func (s *Suite) checkWallet(u *TestUser, when string) {
	l := u.Ledger
	var wallet map[string]any
	s.r.Check(u.Label+" wallet "+when+": totals and lines match ledger", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v3/wallet", u.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		wallet = getMap(resp, "data")
		var problems []string
		if got, want := getNum(wallet, "balance"), l.balance(); !eq(got, want) {
			problems = append(problems, fmt.Sprintf("balance=%.2f want %.2f", got, want))
		}
		if got, want := getNum(wallet, "escrow_amount"), sum(l.held); !eq(got, want) {
			problems = append(problems, fmt.Sprintf("escrow_amount=%.2f want %.2f", got, want))
		}

		lines := map[string][]float64{}
		seen := map[string]bool{}
		for _, t := range getRows(wallet, "transactions") {
			id := getStr(t, "id")
			if seen[id] {
				problems = append(problems, "duplicate transaction line "+id)
			}
			seen[id] = true
			lines[getStr(t, "description")] = append(lines[getStr(t, "description")], getNum(t, "amount"))
		}
		var paidOpen, paidRefunded []float64
		for tx, total := range l.paid {
			if l.refunded[tx] {
				paidRefunded = append(paidRefunded, total)
			} else {
				paidOpen = append(paidOpen, total)
			}
		}
		for _, c := range []struct {
			desc string
			want []float64
		}{
			{"Payment received (held in escrow)", sortedValues(l.held)},
			{"Escrow released", sortedValues(l.released)},
			{"Withdrawal to bank", append([]float64{}, l.withdrawals...)},
			{"Contract payment sent", paidOpen},
			{"Contract payment sent (refunded)", paidRefunded},
		} {
			if want := c.want; !sameAmounts(append([]float64{}, lines[c.desc]...), want) {
				problems = append(problems, fmt.Sprintf("%q lines=%v want %v", c.desc, lines[c.desc], want))
			}
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return fmt.Sprintf("balance=%.2f escrow=%.2f lines=%d", l.balance(), sum(l.held), len(seen)), nil
	})

	if u.StripeAccountID == "" || wallet == nil {
		return
	}
	// The regression for the "available balance £0.80 but withdrawal refused"
	// report: the withdrawable figure must come from Stripe, agree with the
	// payout-balance endpoint, and Stripe's total must equal the ledger.
	s.r.Check(u.Label+" wallet "+when+": available_to_withdraw is Stripe's real balance", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v3/wallet/payout-balance", u.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		pbAvail, pbPending := getNum(resp, "data", "available"), getNum(resp, "data", "pending")
		if _, ok := wallet["available_to_withdraw"]; !ok {
			return "", fmt.Errorf("wallet response has no available_to_withdraw field")
		}
		wAvail, wPending := getNum(wallet, "available_to_withdraw"), getNum(wallet, "pending_settlement")
		if !eq(wAvail, pbAvail) || !eq(wPending, pbPending) {
			return "", fmt.Errorf("wallet says available=%.2f pending=%.2f, payout-balance says %.2f/%.2f", wAvail, wPending, pbAvail, pbPending)
		}
		if total := round2(wAvail + wPending); !eq(total, l.balance()) {
			return "", fmt.Errorf("stripe holds %.2f (available %.2f + pending %.2f) but ledger balance is %.2f", total, wAvail, wPending, l.balance())
		}
		return fmt.Sprintf("available=%.2f pending=%.2f == ledger %.2f", wAvail, wPending, l.balance()), nil
	})
}

func (s *Suite) availableToWithdraw(u *TestUser) (float64, error) {
	status, resp, err := s.api.Do("GET", "/api/v3/wallet", u.AccessToken, nil, nil)
	if err != nil {
		return 0, err
	}
	if err := expect(status, resp, 200); err != nil {
		return 0, err
	}
	return getNum(resp, "data", "available_to_withdraw"), nil
}

func (s *Suite) withdraw(u *TestUser, body map[string]any) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/v3/wallet/withdraw", u.AccessToken, body, nil)
}

func (s *Suite) requireAdmin(name string) bool {
	if s.adminToken == "" {
		s.r.Skip(name, "no admin (pass -db or -admin-token)")
		return false
	}
	return true
}

func (s *Suite) adminEscrowRow(contractID string) (map[string]any, error) {
	status, resp, err := s.api.Do("GET", "/api/v3/admin/payouts/escrows?limit=100", s.adminToken, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := expect(status, resp, 200); err != nil {
		return nil, err
	}
	for _, row := range getRows(resp, "data") {
		if getStr(row, "contract", "id") == contractID {
			return row, nil
		}
	}
	return nil, fmt.Errorf("contract %s not in admin escrow list", contractID)
}

func (s *Suite) adminStats() (map[string]any, error) {
	status, resp, err := s.api.Do("GET", "/api/v3/admin/payments/stats?days=1", s.adminToken, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := expect(status, resp, 200); err != nil {
		return nil, err
	}
	return getMap(resp, "data", "summary"), nil
}
