package main

import (
	"fmt"
	"strings"
	"time"
)

// mainFlow: contract -> payment (declined first, then paid) -> wallet ->
// release -> withdrawal, with every refusal path along the way.
func (s *Suite) mainFlow(client, f1, outsider *TestUser) *Flow {
	s.r.Section("Main flow — contract and payment intent")
	a := s.setupFlow("A", client, f1, 20)

	s.r.Check("duplicate contract for the same proposal is refused", func() (string, error) {
		status, resp, err := s.createContract(client.AccessToken, a.ProposalID, 20, time.Now(), time.Now().Add(24*time.Hour))
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "already exists")
	})

	intentRefused := func(name, token string, body map[string]any, want int) {
		s.r.Check(name, func() (string, error) {
			status, resp, err := s.api.Do("POST", "/api/v3/payments/intent", token, body, nil)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d: %s", status, errMsg(resp)), expect(status, resp, want)
		})
	}
	intentRefused("intent without proposal_id is 400", client.AccessToken, map[string]any{}, 400)
	intentRefused("intent for unknown proposal is 404", client.AccessToken, map[string]any{"proposal_id": "00000000-0000-0000-0000-000000000000"}, 404)
	intentRefused("freelancer cannot pay their own proposal (403)", f1.AccessToken, map[string]any{"proposal_id": a.ProposalID}, 403)
	intentRefused("outsider cannot pay the proposal (403)", outsider.AccessToken, map[string]any{"proposal_id": a.ProposalID}, 403)

	s.r.MustCheck("client creates payment intent; server computes the charge", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, a.ProposalID, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		a.PaymentIntentID = getStr(resp, "data", "payment_intent_id")
		if got := getNum(resp, "data", "amount"); !eq(got, a.Fees.ClientTotal) {
			return "", fmt.Errorf("amount £%.2f, expected £%.2f", got, a.Fees.ClientTotal)
		}
		sum := getMap(resp, "data", "payment_summary")
		if got := getNum(sum, "freelancer", "amount_to_receive"); !eq(got, a.Fees.FreelancerNet) {
			return "", fmt.Errorf("summary freelancer net £%.2f, expected £%.2f", got, a.Fees.FreelancerNet)
		}
		return fmt.Sprintf("%s £%.2f", a.PaymentIntentID, a.Fees.ClientTotal), nil
	})
	s.r.Check("client-supplied amount is ignored (no tampering)", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, a.ProposalID, map[string]any{"amount": 0.01})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if got := getNum(resp, "data", "amount"); !eq(got, a.Fees.ClientTotal) {
			return "", fmt.Errorf("charge changed to £%.2f", got)
		}
		return fmt.Sprintf("still £%.2f", a.Fees.ClientTotal), nil
	})
	s.r.Check("repeat intent request returns the same PaymentIntent (no second charge)", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, a.ProposalID, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if got := getStr(resp, "data", "payment_intent_id"); got != a.PaymentIntentID {
			return "", fmt.Errorf("got a new PaymentIntent %s (first was %s) — a retry can double-charge", got, a.PaymentIntentID)
		}
		return "same " + a.PaymentIntentID, nil
	})
	s.r.Check("stripe PaymentIntent amount, currency, customer and metadata", func() (string, error) {
		pi, err := s.sc.Get("/payment_intents/"+a.PaymentIntentID, "")
		if err != nil {
			return "", err
		}
		var problems []string
		if got := int64(getNum(pi, "amount")); got != pence(a.Fees.ClientTotal) {
			problems = append(problems, fmt.Sprintf("amount=%dp", got))
		}
		if getStr(pi, "currency") != "gbp" {
			problems = append(problems, "currency="+getStr(pi, "currency"))
		}
		if getStr(pi, "customer") == "" {
			problems = append(problems, "no customer linked")
		}
		md := getMap(pi, "metadata")
		for k, want := range map[string]string{
			"fee_version":    "2",
			"proposal_id":    a.ProposalID,
			"from_user_id":   client.ID,
			"to_user_id":     f1.ID,
			"freelancer_net": fmt.Sprint(pence(a.Fees.FreelancerNet)),
			"client_total":   fmt.Sprint(pence(a.Fees.ClientTotal)),
		} {
			if got := getStr(md, k); got != want {
				problems = append(problems, fmt.Sprintf("metadata.%s=%q want %q", k, got, want))
			}
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return "customer=" + getStr(pi, "customer"), nil
	})

	s.r.Check("declined card fails at Stripe", func() (string, error) {
		_, err := s.confirmIntent(a.PaymentIntentID, pmDeclined)
		if err == nil {
			return "", fmt.Errorf("declined test card was accepted")
		}
		return err.Error(), nil
	})
	s.r.Check("declined payment records nothing", func() (string, error) {
		time.Sleep(6 * time.Second) // give any (wrong) webhook time to land
		rows, _, err := s.contractTransactions(client.AccessToken, a.ContractID)
		if err != nil {
			return "", err
		}
		if len(rows) != 0 {
			return "", fmt.Errorf("%d transaction(s) recorded for a declined payment", len(rows))
		}
		return "0 transactions", nil
	})
	s.r.MustCheck("retry the same intent with a good card succeeds", func() (string, error) {
		pi, err := s.confirmIntent(a.PaymentIntentID, pmBypassPending)
		if err != nil {
			return "", err
		}
		if st := getStr(pi, "status"); st != "succeeded" {
			return "", fmt.Errorf("status %s", st)
		}
		a.ChargeID = getStr(pi, "latest_charge")
		return "charge=" + a.ChargeID, nil
	})
	s.r.MustCheck("charge.succeeded webhook records a held payment", func() (string, error) {
		tx, err := s.waitForStatus(client.AccessToken, a.ContractID, "held", 30*time.Second)
		if err != nil {
			return "", err
		}
		a.TransactionID = getStr(tx, "id")
		if !eq(getNum(tx, "gross_amount"), a.Fees.ClientTotal) || !eq(getNum(tx, "net_amount"), a.Fees.FreelancerNet) ||
			!eq(getNum(tx, "platform_fee"), a.Fees.PlatformEarnings) {
			return "", fmt.Errorf("gross/net/platform = %.2f/%.2f/%.2f, want %.2f/%.2f/%.2f",
				getNum(tx, "gross_amount"), getNum(tx, "net_amount"), getNum(tx, "platform_fee"),
				a.Fees.ClientTotal, a.Fees.FreelancerNet, a.Fees.PlatformEarnings)
		}
		return fmt.Sprintf("gross=%.2f net=%.2f platform=%.2f", a.Fees.ClientTotal, a.Fees.FreelancerNet, a.Fees.PlatformEarnings), nil
	})
	f1.Ledger.hold(a.ContractID, a.Fees.FreelancerNet)
	client.Ledger.pay(a.TransactionID, a.Fees.ClientTotal)
	s.checkPaymentRow(a, "held")

	s.r.Check("paying an already-paid proposal is refused (409)", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, a.ProposalID, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 409); err != nil {
			return "", err
		}
		return errMsg(resp), nil
	})
	s.r.Check("exactly one payment recorded for the contract", func() (string, error) {
		time.Sleep(3 * time.Second)
		rows, resp, err := s.contractTransactions(client.AccessToken, a.ContractID)
		if err != nil {
			return "", err
		}
		if len(rows) != 1 {
			return "", fmt.Errorf("%d payments for one contract", len(rows))
		}
		return fmt.Sprintf("1 payment; client total_paid=%.2f", getNum(resp, "summary", "total_paid")), nil
	})

	s.r.Section("Main flow — wallets while money is in escrow")
	s.checkWallet(f1, "while held")
	s.checkWallet(client, "after paying")

	s.r.Section("Main flow — release rules")
	s.r.Check("release before completion is refused (409)", func() (string, error) {
		status, resp, err := s.releaseContract(client, a.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	s.r.Check("outsider cannot release (403)", func() (string, error) {
		status, resp, err := s.releaseContract(outsider, a.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 403)
	})
	s.r.Check("outsider cannot complete the contract (403)", func() (string, error) {
		status, resp, err := s.complete(outsider, a.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 403)
	})
	s.r.Check("freelancer completes; payment not released yet", func() (string, error) {
		status, resp, err := s.complete(f1, a.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if st := getStr(resp, "payment_release", "status"); st != "not_applicable" {
			return "", fmt.Errorf("payment_release=%s after only one side completed", st)
		}
		return "payment_release=not_applicable", nil
	})
	s.r.Check("release with only one side complete is refused (409)", func() (string, error) {
		status, resp, err := s.releaseContract(f1, a.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	s.checkPaymentRow(a, "held")

	s.r.MustCheck("client completes; escrow auto-releases", func() (string, error) {
		status, resp, err := s.complete(client, a.ContractID)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		pr := getMap(resp, "payment_release")
		if getStr(pr, "status") != "released" || getNum(pr, "released") != 1 {
			return "", fmt.Errorf("payment_release=%v", pr)
		}
		return "payment_release=released (1 escrow)", nil
	})
	f1.Ledger.release(a.ContractID)
	s.r.Check("completing again is refused", func() (string, error) {
		status, resp, err := s.complete(client, a.ContractID)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "already")
	})
	s.r.Check("second release by contract is refused (no double payout)", func() (string, error) {
		status, resp, err := s.releaseContract(f1, a.ContractID)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})
	s.checkPaymentRow(a, "released")
	s.ensureEscrowID(a)
	if a.EscrowID != "" {
		s.r.Check("second release by escrow id is refused", func() (string, error) {
			status, resp, err := s.api.Do("POST", "/api/v3/escrow/"+a.EscrowID+"/release", client.AccessToken, nil, nil)
			if err != nil {
				return "", err
			}
			return errMsg(resp), expect(status, resp, 409)
		})
	}
	s.verifyTransfer(a)
	s.r.Check("transaction list shows released", func() (string, error) {
		rows, _, err := s.contractTransactions(client.AccessToken, a.ContractID)
		if err != nil {
			return "", err
		}
		if len(rows) != 1 || getStr(rows[0], "status") != "released" {
			return "", fmt.Errorf("rows=%d status=%s", len(rows), getStr(rows[0], "status"))
		}
		return "released", nil
	})

	s.r.Section("Main flow — wallets after release")
	s.checkWallet(f1, "after release")
	s.checkWallet(client, "after release")

	s.r.Section("Main flow — withdrawal")
	s.withdrawalRules(f1, client, a)
	return a
}

func (s *Suite) withdrawalRules(f1, client *TestUser, a *Flow) {
	avail, err := s.availableToWithdraw(f1)
	if err != nil {
		s.r.Check("read available_to_withdraw", func() (string, error) { return "", err })
		return
	}
	s.r.Warn("released funds are withdrawable now (bypassPending card)", func() (string, error) {
		if !eq(avail, a.Fees.FreelancerNet) {
			return "", fmt.Errorf("available_to_withdraw=%.2f, released=%.2f — Stripe still holding funds; success path below is limited", avail, a.Fees.FreelancerNet)
		}
		return fmt.Sprintf("available_to_withdraw=%.2f", avail), nil
	})

	refused := func(name string, u *TestUser, body map[string]any, contains string) {
		s.r.Check(name, func() (string, error) {
			status, resp, err := s.withdraw(u, body)
			if err != nil {
				return "", err
			}
			return expectRefused(status, resp, contains)
		})
	}
	refused("withdraw 0 is refused", f1, map[string]any{"amount": 0}, "greater than zero")
	refused("negative withdraw is refused", f1, map[string]any{"amount": -5}, "greater than zero")
	refused("withdraw more than available_to_withdraw is refused", f1, map[string]any{"amount": round2(avail + 0.01)}, "exceeds")
	refused("withdraw to another user's bank account is refused", f1, map[string]any{"amount": 1, "bank_account_id": client.BankID}, "not found")
	refused("withdraw to an unknown bank account is refused", f1, map[string]any{"amount": 1, "bank_account_id": "00000000-0000-0000-0000-000000000000"}, "not found")
	refused("client with nothing available cannot withdraw", client, map[string]any{}, "no available balance")
	refused("wrong-typed amount is refused", f1, map[string]any{"amount": "all"}, "")

	if avail < 1 {
		s.r.Warn("withdrawal success path", func() (string, error) {
			return "", fmt.Errorf("skipped: only £%.2f available", avail)
		})
		return
	}

	var w1, w2 string
	s.r.Check("partial withdrawal of £1.00 to a chosen bank account", func() (string, error) {
		status, resp, err := s.withdraw(f1, map[string]any{"amount": 1.00, "bank_account_id": f1.BankID, "currency": "gbp"})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		d := getMap(resp, "data")
		w1 = getStr(d, "stripe_payout_id")
		if !eq(getNum(d, "amount"), 1) || w1 == "" || getStr(d, "status") == "failed" {
			return "", fmt.Errorf("withdrawal=%v", d)
		}
		return fmt.Sprintf("payout=%s status=%s bank=…%s", w1, getStr(d, "status"), getStr(d, "bank_last4")), nil
	})
	if w1 != "" {
		f1.Ledger.withdraw(1)
		s.r.Check("stripe payout amount and destination match", func() (string, error) {
			p, err := s.sc.Get("/payouts/"+w1, f1.StripeAccountID)
			if err != nil {
				return "", err
			}
			if int64(getNum(p, "amount")) != 100 || getStr(p, "destination") != f1.BankStripeID {
				return "", fmt.Errorf("amount=%v destination=%s (want 100 -> %s)", getNum(p, "amount"), getStr(p, "destination"), f1.BankStripeID)
			}
			return "100p -> " + f1.BankStripeID, nil
		})
		s.checkWallet(f1, "after partial withdrawal")
	}

	rest := round2(avail - 1)
	s.r.Check("withdraw everything left (no amount given)", func() (string, error) {
		status, resp, err := s.withdraw(f1, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		d := getMap(resp, "data")
		w2 = getStr(d, "stripe_payout_id")
		if !eq(getNum(d, "amount"), rest) {
			return "", fmt.Errorf("withdrew £%.2f, expected the remaining £%.2f", getNum(d, "amount"), rest)
		}
		return fmt.Sprintf("£%.2f payout=%s", rest, w2), nil
	})
	if w2 != "" {
		f1.Ledger.withdraw(rest)
		s.checkWallet(f1, "after full withdrawal")
	}
	refused("withdraw with nothing left is refused", f1, map[string]any{}, "no available balance")

	s.r.Check("withdrawal history lists both payouts", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v3/wallet/withdrawals", f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		var amounts []float64
		for _, w := range getRows(resp, "data") {
			amounts = append(amounts, getNum(w, "amount"))
		}
		if !sameAmounts(amounts, []float64{1, rest}) {
			return "", fmt.Errorf("history amounts=%v want [1 %.2f]", amounts, rest)
		}
		return fmt.Sprintf("%v", amounts), nil
	})
	s.r.Warn("payout webhooks update withdrawal status", func() (string, error) {
		return pollUntil(60*time.Second, func() (string, bool, error) {
			status, resp, err := s.api.Do("GET", "/api/v3/wallet/withdrawals", f1.AccessToken, nil, nil)
			if err != nil {
				return "", false, err
			}
			if err := expect(status, resp, 200); err != nil {
				return "", false, err
			}
			var sts []string
			done := true
			for _, w := range getRows(resp, "data") {
				st := getStr(w, "status")
				sts = append(sts, st)
				if st == "failed" || st == "canceled" {
					return "", false, fmt.Errorf("payout %s", st)
				}
				if st != "paid" {
					done = false
				}
			}
			return strings.Join(sts, ","), done, nil
		})
	})
}
