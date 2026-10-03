package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *Suite) ensureEscrowID(f *Flow) {
	if f.EscrowID != "" || s.adminToken == "" {
		return
	}
	if row, err := s.adminEscrowRow(f.ContractID); err == nil {
		f.EscrowID = getStr(row, "id")
	}
}

// unpayableFreelancerFlow reproduces the live incident: a contract completes
// while the freelancer's Connect account can't receive transfers. Release
// must fail without losing or misreporting money, and succeed once the
// freelancer finishes onboarding.
func (s *Suite) unpayableFreelancerFlow(client, f2 *TestUser) *Flow {
	s.r.Section("Unpayable freelancer — release fails safely, then recovers")
	s.r.MustCheck("verify identity: freelancer2", func() (string, error) { return s.verifyIdentity(f2) })
	if s.inProgressJobID != "" {
		s.r.Check("proposal on a job already in progress is refused", func() (string, error) {
			status, resp, err := s.sendProposal(f2, s.inProgressJobID, 20, nil)
			if err != nil {
				return "", err
			}
			return expectRefused(status, resp, "not accepting")
		})
	}

	d := s.setupFlow("D", client, f2, 10)
	s.r.Check("freelancer2 connect status is not ready", func() (string, error) {
		st, err := s.connectStatus(f2)
		if err != nil {
			return "", err
		}
		if getStr(st, "status") == "ready" {
			return "", fmt.Errorf("account unexpectedly ready")
		}
		return fmt.Sprintf("status=%s missing=%v", getStr(st, "status"), getSlice(st, "missing")), nil
	})
	s.payFlow(d, pmVisa)
	s.checkWallet(f2, "while held")

	s.r.Check("freelancer2 completes", func() (string, error) {
		status, resp, err := s.complete(f2, d.ContractID)
		if err != nil {
			return "", err
		}
		return getStr(resp, "payment_release", "status"), expect(status, resp, 200)
	})
	s.r.Check("client completes; release reports failure, completion still succeeds", func() (string, error) {
		status, resp, err := s.complete(client, d.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		pr := getMap(resp, "payment_release")
		if getStr(pr, "status") != "failed" || !strings.Contains(getStr(pr, "error"), "capabilit") {
			return "", fmt.Errorf("payment_release=%v (want failed + capability error)", pr)
		}
		return "payment_release=failed: insufficient capabilities", nil
	})
	s.checkPaymentRow(d, "held")
	s.noTransfer(d)
	s.checkWallet(f2, "after failed release")

	s.r.Check("retrying release still fails with the capability error", func() (string, error) {
		status, resp, err := s.releaseContract(f2, d.ContractID)
		if err != nil {
			return "", err
		}
		if status == 200 || !strings.Contains(errMsg(resp), "capabilit") {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		return fmt.Sprintf("%d (escrow stays held)", status), nil
	})
	if s.adminToken != "" {
		s.r.Warn("admin escrow list flags the freelancer as not payable", func() (string, error) {
			row, err := s.adminEscrowRow(d.ContractID)
			if err != nil {
				return "", err
			}
			if getBool(row, "can_release") {
				return "", fmt.Errorf("can_release=true, but Stripe will refuse the transfer (list only checks that an account id exists)")
			}
			return "can_release=false: " + getStr(row, "block_reason"), nil
		})
		s.ensureEscrowID(d)
		s.r.Check("admin force-release also fails safely", func() (string, error) {
			status, resp, err := s.api.Do("POST", "/api/v3/admin/payouts/escrows/"+d.EscrowID+"/release", s.adminToken, map[string]any{"note": "paymenttest"}, nil)
			if err != nil {
				return "", err
			}
			if status == 200 {
				return "", fmt.Errorf("admin release reported success with no payable account")
			}
			return fmt.Sprintf("%d: %s", status, errMsg(resp)), nil
		})
	}
	s.checkPaymentRow(d, "held")

	s.r.Section("Stripe Connect onboarding — freelancer2 (recovery)")
	s.onboard(f2, "Test", "Freelancer2", "")

	s.r.MustCheck("after onboarding, retrying release pays out", func() (string, error) {
		status, resp, err := s.releaseContract(f2, d.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		return fmt.Sprintf("released=%v", resp["released"]), nil
	})
	f2.Ledger.release(d.ContractID)
	s.checkPaymentRow(d, "released")
	s.verifyTransfer(d)
	s.checkWallet(f2, "after recovery")
	d.FinalStatus = "released"
	return d
}

func (s *Suite) disputeFlow(client, f1 *TestUser, a *Flow) *Flow {
	s.r.Section("Dispute gate")
	c := s.setupFlow("C", client, f1, 15)
	s.payFlow(c, pmBypassPending)
	c.FinalStatus = "held"

	s.r.Check("returning client reuses the same Stripe customer", func() (string, error) {
		pa, err := s.sc.Get("/payment_intents/"+a.PaymentIntentID, "")
		if err != nil {
			return "", err
		}
		pc, err := s.sc.Get("/payment_intents/"+c.PaymentIntentID, "")
		if err != nil {
			return "", err
		}
		if getStr(pa, "customer") == "" || getStr(pa, "customer") != getStr(pc, "customer") {
			return "", fmt.Errorf("customers differ: %s vs %s", getStr(pa, "customer"), getStr(pc, "customer"))
		}
		return getStr(pc, "customer"), nil
	})

	s.r.Check("dispute with too-short description is refused", func() (string, error) {
		status, resp, err := s.api.PostForm("/api/v1/disputes", f1.AccessToken, url.Values{
			"contract_id": {c.ContractID}, "reason": {"x"}, "description": {"short"},
		}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})
	var disputeID string
	s.r.MustCheck("freelancer files a dispute", func() (string, error) {
		status, resp, err := s.api.PostForm("/api/v1/disputes", f1.AccessToken, url.Values{
			"contract_id": {c.ContractID},
			"reason":      {"Client unresponsive"},
			"description": {"Client stopped responding after the job was done (paymenttest)"},
		}, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		disputeID = getStr(resp, "data", "id")
		return disputeID, nil
	})

	for _, u := range []*TestUser{client, f1} {
		u := u
		s.r.Check(u.Label+" cannot complete while disputed", func() (string, error) {
			status, resp, err := s.complete(u, c.ContractID)
			if err != nil {
				return "", err
			}
			return expectRefused(status, resp, "dispute")
		})
	}
	s.r.Check("release is blocked while disputed (409)", func() (string, error) {
		status, resp, err := s.releaseContract(f1, c.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	if s.adminToken != "" {
		s.ensureEscrowID(c)
		s.r.Check("admin force-release is blocked while disputed (409)", func() (string, error) {
			status, resp, err := s.api.Do("POST", "/api/v3/admin/payouts/escrows/"+c.EscrowID+"/release", s.adminToken, map[string]any{"note": "paymenttest override"}, nil)
			if err != nil {
				return "", err
			}
			return errMsg(resp), expect(status, resp, 409)
		})
	}
	s.checkPaymentRow(c, "held")

	resolved := false
	s.r.Check("a party to the dispute cannot resolve it (admin only)", func() (string, error) {
		status, resp, err := s.api.Do("PUT", "/api/v1/admin/disputes/"+disputeID+"/resolve", client.AccessToken, map[string]any{
			"resolution": "client tries to close their own dispute",
		}, nil)
		if err != nil {
			return "", err
		}
		if status == 200 {
			resolved = true
			return "", fmt.Errorf("the client resolved the dispute against them — the dispute gate can be bypassed")
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})
	if !resolved {
		if !s.requireAdmin("admin resolves the dispute") {
			return c
		}
		s.r.MustCheck("admin resolves the dispute", func() (string, error) {
			status, resp, err := s.api.Do("PUT", "/api/v1/admin/disputes/"+disputeID+"/resolve", s.adminToken, map[string]any{
				"resolution": "paymenttest: work verified, release payment",
			}, nil)
			if err != nil {
				return "", err
			}
			return "resolved", expect(status, resp, 200)
		})
	}

	s.r.MustCheck("after resolution, completion releases payment", func() (string, error) {
		if status, resp, err := s.complete(f1, c.ContractID); err != nil || status != 200 {
			return "", fmt.Errorf("freelancer complete: %d %v %v", status, err, errMsg(resp))
		}
		status, resp, err := s.complete(client, c.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if st := getStr(resp, "payment_release", "status"); st != "released" {
			return "", fmt.Errorf("payment_release=%v", resp["payment_release"])
		}
		return "released", nil
	})
	f1.Ledger.release(c.ContractID)
	c.FinalStatus = "released"
	s.checkPaymentRow(c, "released")
	s.verifyTransfer(c)
	s.checkWallet(f1, "after dispute release")
	return c
}

func (s *Suite) adminRefund(contractID, reason string) (int, map[string]any, error) {
	body := map[string]any{}
	if reason != "" {
		body["reason"] = reason
	}
	return s.api.Do("POST", "/api/v3/admin/payouts/contracts/"+contractID+"/refund", s.adminToken, body, nil)
}

func (s *Suite) stripeRefunds(piID string) ([]map[string]any, error) {
	list, err := s.sc.Get("/refunds?limit=10&payment_intent="+piID, "")
	if err != nil {
		return nil, err
	}
	return getRows(list, "data"), nil
}

func (s *Suite) checkStripeRefund(f *Flow) {
	s.r.Check(f.Label+": stripe refund is the full client charge", func() (string, error) {
		refunds, err := s.stripeRefunds(f.PaymentIntentID)
		if err != nil {
			return "", err
		}
		if len(refunds) != 1 {
			return "", fmt.Errorf("%d refunds on the PaymentIntent (want 1)", len(refunds))
		}
		r := refunds[0]
		if int64(getNum(r, "amount")) != pence(f.Fees.ClientTotal) || getStr(r, "status") != "succeeded" {
			return "", fmt.Errorf("refund amount=%v status=%s, want %dp succeeded", getNum(r, "amount"), getStr(r, "status"), pence(f.Fees.ClientTotal))
		}
		return fmt.Sprintf("%s %dp succeeded", getStr(r, "id"), pence(f.Fees.ClientTotal)), nil
	})
}

func (s *Suite) checkRefundRow(f *Flow, wantReversal bool) {
	name := f.Label + ": DB records the refund with audit trail"
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
			return "", fmt.Errorf("%d payments / %d escrows", len(pays), len(escs))
		}
		p, e := pays[0], escs[0]
		var problems []string
		if p.Status != "refunded" || e.Status != "refunded" {
			problems = append(problems, fmt.Sprintf("status payment=%s escrow=%s", p.Status, e.Status))
		}
		if p.StripeRefundID == "" {
			problems = append(problems, "no stripe_refund_id")
		}
		if s.admin != nil && p.RefundedByAdminID != s.admin.ID {
			problems = append(problems, "refunded_by_admin_id="+p.RefundedByAdminID)
		}
		if p.RefundReason == "" {
			problems = append(problems, "no refund_reason")
		}
		if (p.StripeReversalID != "") != wantReversal {
			problems = append(problems, fmt.Sprintf("stripe_reversal_id=%q (reversal expected: %v)", p.StripeReversalID, wantReversal))
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return fmt.Sprintf("refund=%s reversal=%s by=%s", p.StripeRefundID, p.StripeReversalID, p.RefundedByAdminID), nil
	})
}

func (s *Suite) refundHeldFlow(client, f1 *TestUser) *Flow {
	s.r.Section("Admin refund while money is in escrow")
	e := s.setupFlow("E", client, f1, 8)
	s.payFlow(e, pmVisa)
	e.FinalStatus = "held"
	s.checkWallet(f1, "with E held")

	s.r.Check("non-admin cannot refund (403)", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/admin/payouts/contracts/"+e.ContractID+"/refund", client.AccessToken, map[string]any{"reason": "x"}, nil)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})
	if !s.requireAdmin("admin refund scenarios") {
		return e
	}
	s.r.Check("refund without a reason is refused (400)", func() (string, error) {
		status, resp, err := s.adminRefund(e.ContractID, "")
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 400)
	})

	before, statsErr := s.adminStats()
	s.r.MustCheck("admin refunds the held payment", func() (string, error) {
		status, resp, err := s.adminRefund(e.ContractID, "paymenttest: client cancelled before work started")
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if getStr(resp, "data", "status") != "refunded" || getStr(resp, "data", "stripe_reversal_id") != "" {
			return "", fmt.Errorf("status=%s reversal=%q", getStr(resp, "data", "status"), getStr(resp, "data", "stripe_reversal_id"))
		}
		return "refunded, no transfer to reverse", nil
	})
	f1.Ledger.dropHeld(e.ContractID)
	client.Ledger.refund(e.TransactionID)
	e.FinalStatus = "refunded"
	s.checkStripeRefund(e)
	s.checkRefundRow(e, false)

	s.r.Check("admin stats drop the refunded payment from volume, fees and net", func() (string, error) {
		if statsErr != nil {
			return "", statsErr
		}
		after, err := s.adminStats()
		if err != nil {
			return "", err
		}
		var problems []string
		for _, c := range []struct {
			key  string
			want float64
		}{{"total_volume", e.Fees.ClientTotal}, {"total_fees", e.Fees.PlatformEarnings}, {"total_net", e.Fees.FreelancerNet}} {
			if drop := round2(getNum(before, c.key) - getNum(after, c.key)); !eq(drop, c.want) {
				problems = append(problems, fmt.Sprintf("%s dropped by %.2f, want %.2f", c.key, drop, c.want))
			}
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return fmt.Sprintf("volume -%.2f fees -%.2f net -%.2f", e.Fees.ClientTotal, e.Fees.PlatformEarnings, e.Fees.FreelancerNet), nil
	})
	s.r.Check("second refund is refused (409)", func() (string, error) {
		status, resp, err := s.adminRefund(e.ContractID, "again")
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	s.r.Check("refund by transaction id is also refused (409)", func() (string, error) {
		status, resp, err := s.api.Do("POST", "/api/v3/admin/payouts/transactions/"+e.TransactionID+"/refund", s.adminToken, map[string]any{"reason": "again"}, nil)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	s.r.Check("completing a refunded contract releases nothing", func() (string, error) {
		if status, resp, err := s.complete(f1, e.ContractID); err != nil || status != 200 {
			return "", fmt.Errorf("freelancer complete: %d %v %s", status, err, errMsg(resp))
		}
		status, resp, err := s.complete(client, e.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if st := getStr(resp, "payment_release", "status"); st != "no_payment_held" {
			return "", fmt.Errorf("payment_release=%v", resp["payment_release"])
		}
		return "payment_release=no_payment_held", nil
	})
	s.noTransfer(e)
	s.checkRefundRow(e, false)
	s.checkWallet(f1, "after held refund")
	s.checkWallet(client, "after held refund")
	return e
}

// refundReleasedFlows covers refunds after money already moved: C (released,
// still in the freelancer's Stripe balance) and A (released and withdrawn).
func (s *Suite) refundReleasedFlows(client, f1 *TestUser, c, a *Flow) {
	s.r.Section("Admin refund after release (transfer reversal)")
	if !s.requireAdmin("refund after release") || c.FinalStatus != "released" {
		return
	}
	s.r.Check("admin refunds a released payment; transfer is reversed first", func() (string, error) {
		status, resp, err := s.adminRefund(c.ContractID, "paymenttest: dispute later upheld for client")
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if getStr(resp, "data", "stripe_reversal_id") == "" {
			return "", fmt.Errorf("no stripe_reversal_id on a released refund")
		}
		return "reversal=" + getStr(resp, "data", "stripe_reversal_id"), nil
	})
	f1.Ledger.dropReleased(c.ContractID)
	client.Ledger.refund(c.TransactionID)
	c.FinalStatus = "refunded"
	s.r.Check("C: stripe transfer fully reversed", func() (string, error) {
		t, err := s.sc.Get("/transfers/"+c.TransferID, "")
		if err != nil {
			return "", err
		}
		if !getBool(t, "reversed") || int64(getNum(t, "amount_reversed")) != pence(c.Fees.FreelancerNet) {
			return "", fmt.Errorf("reversed=%v amount_reversed=%v", getBool(t, "reversed"), getNum(t, "amount_reversed"))
		}
		return fmt.Sprintf("%dp reversed", pence(c.Fees.FreelancerNet)), nil
	})
	s.checkStripeRefund(c)
	s.checkRefundRow(c, true)
	s.checkWallet(f1, "after refunding released C")
	s.checkWallet(client, "after refunding released C")

	s.r.Section("Admin refund after the freelancer already withdrew")
	var refundedAfterWithdrawal bool
	s.r.Check("refund of withdrawn payment A is applied consistently", func() (string, error) {
		status, resp, err := s.adminRefund(a.ContractID, "paymenttest: refund after withdrawal")
		if err != nil {
			return "", err
		}
		refunds, rerr := s.stripeRefunds(a.PaymentIntentID)
		if rerr != nil {
			return "", rerr
		}
		if status == 200 {
			refundedAfterWithdrawal = true
			if len(refunds) != 1 {
				return "", fmt.Errorf("app says refunded but stripe has %d refunds", len(refunds))
			}
			return "refunded (transfer reversed into a negative freelancer balance)", nil
		}
		if len(refunds) != 0 {
			return "", fmt.Errorf("app refused (%d) but stripe refunded the client anyway", status)
		}
		return fmt.Sprintf("refused %d: %s", status, errMsg(resp)), nil
	})
	if refundedAfterWithdrawal {
		f1.Ledger.dropReleased(a.ContractID)
		client.Ledger.refund(a.TransactionID)
		a.FinalStatus = "refunded"
		s.checkRefundRow(a, true)
		s.r.Warn("policy: refunds after withdrawal leave the freelancer's Stripe balance negative", func() (string, error) {
			return "", fmt.Errorf("the platform carries this loss until the freelancer earns again — decide whether admins should be allowed to do this")
		})
	} else {
		a.FinalStatus = "released"
		s.checkPaymentRow(a, "released")
	}
	s.checkWallet(f1, "after refund attempt on withdrawn A")
	s.checkWallet(client, "after refund attempt on withdrawn A")
}

func (s *Suite) finalInvariants(client, f1, f2 *TestUser, flows []*Flow) {
	s.r.Section("Final invariants")
	for _, f := range flows {
		if f.FinalStatus == "" || f.FinalStatus == "refunded" {
			continue // refunded rows were verified by checkRefundRow
		}
		s.checkPaymentRow(f, f.FinalStatus)
	}
	if s.adminToken != "" {
		s.r.Check("admin escrow list shows every flow with the right status", func() (string, error) {
			var problems []string
			for _, f := range flows {
				row, err := s.adminEscrowRow(f.ContractID)
				if err != nil {
					problems = append(problems, f.Label+": "+err.Error())
					continue
				}
				if st := getStr(row, "status"); st != f.FinalStatus {
					problems = append(problems, fmt.Sprintf("%s: %s want %s", f.Label, st, f.FinalStatus))
				}
			}
			if len(problems) > 0 {
				return "", fmt.Errorf("%s", strings.Join(problems, "; "))
			}
			return fmt.Sprintf("%d flows match", len(flows)), nil
		})
		s.r.Check("admin withdrawals list shows the freelancer's payouts", func() (string, error) {
			status, resp, err := s.api.Do("GET", "/api/v3/admin/payouts/withdrawals?limit=100", s.adminToken, nil, nil)
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 200); err != nil {
				return "", err
			}
			n := 0
			for _, w := range getRows(resp, "data") {
				if getStr(w, "user", "id") == f1.ID {
					n++
				}
			}
			if n != len(f1.Ledger.withdrawals) {
				return "", fmt.Errorf("admin sees %d withdrawals, freelancer made %d", n, len(f1.Ledger.withdrawals))
			}
			return fmt.Sprintf("%d withdrawals", n), nil
		})
	}
	s.r.Check("client transaction summary totals exclude refunds", func() (string, error) {
		_, resp, err := s.contractTransactions(client.AccessToken, "")
		if err != nil {
			return "", err
		}
		want := 0.0
		for tx, total := range client.Ledger.paid {
			if !client.Ledger.refunded[tx] {
				want += total
			}
		}
		if got := getNum(resp, "summary", "total_paid"); !eq(got, round2(want)) {
			return "", fmt.Errorf("total_paid=%.2f want %.2f", got, want)
		}
		return fmt.Sprintf("total_paid=%.2f", want), nil
	})
	s.r.Check("freelancer total_earned equals released escrow", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v3/payments/transactions", f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if got, want := getNum(resp, "summary", "total_earned"), sum(f1.Ledger.released); !eq(got, want) {
			return "", fmt.Errorf("total_earned=%.2f want %.2f", got, want)
		}
		return fmt.Sprintf("total_earned=%.2f", sum(f1.Ledger.released)), nil
	})
	time.Sleep(2 * time.Second) // let late webhooks (charge.refunded, payout.*) land before the last read
	s.checkWallet(f1, "final")
	s.checkWallet(f2, "final")
	s.checkWallet(client, "final")
}
