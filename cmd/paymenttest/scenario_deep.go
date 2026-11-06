package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"
)

const pmCreateDispute = "pm_card_createDispute" // succeeds, then Stripe opens a chargeback

type callResult struct {
	status int
	resp   map[string]any
	err    error
}

func parallel(n int, fn func(i int) (int, map[string]any, error)) []callResult {
	var wg sync.WaitGroup
	out := make([]callResult, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			st, resp, err := fn(i)
			out[i] = callResult{st, resp, err}
		}(i)
	}
	close(start)
	wg.Wait()
	return out
}

func statuses(rs []callResult) []int {
	out := make([]int, len(rs))
	for i, r := range rs {
		out[i] = r.status
	}
	return out
}

func (s *Suite) proposalOnly(label string, client, freelancer *TestUser, bid float64, accept bool) *Flow {
	f := &Flow{Label: label, Client: client, Freelancer: freelancer, Fees: computeFees(bid, s.settings)}
	s.r.MustCheck(label+": job and proposal", func() (string, error) {
		id, err := s.createJob(client, "paytest "+label, bid)
		if err != nil {
			return "", err
		}
		f.JobID = id
		status, resp, err := s.sendProposal(freelancer, f.JobID, bid, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200, 201); err != nil {
			return "", err
		}
		f.ProposalID = getStr(resp, "data", "id")
		if accept {
			status, resp, err = s.decide(client.AccessToken, f.ProposalID, "accepted")
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 200); err != nil {
				return "", err
			}
		}
		return f.ProposalID, nil
	})
	return f
}

func (s *Suite) completeBoth(f *Flow) (string, error) {
	if status, resp, err := s.complete(f.Freelancer, f.ContractID); err != nil || status != 200 {
		return "", fmt.Errorf("freelancer complete: %d %v %s", status, err, errMsg(resp))
	}
	status, resp, err := s.complete(f.Client, f.ContractID)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200); err != nil {
		return "", err
	}
	return getStr(resp, "payment_release", "status"), nil
}

func (s *Suite) txStatus(f *Flow) string {
	rows, _, err := s.contractTransactions(f.Client.AccessToken, f.ContractID)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return getStr(rows[0], "status")
}

func (s *Suite) raceFlow(client, fr *TestUser) *Flow {
	s.r.Section("Deep — double taps and races")
	g := s.proposalOnly("G", client, fr, 7, true)

	s.r.Check("5 simultaneous create-contract calls make exactly one contract", func() (string, error) {
		rs := parallel(5, func(int) (int, map[string]any, error) {
			return s.createContract(client.AccessToken, g.ProposalID, 7, time.Now(), time.Now().Add(24*time.Hour))
		})
		created := 0
		for _, r := range rs {
			if r.status == 201 {
				created++
				g.ContractID = getStr(r.resp, "contract", "id")
			}
		}
		n, err := s.contractCount(client.AccessToken, g.ProposalID)
		if err != nil {
			return "", err
		}
		if created != 1 || n != 1 {
			return "", fmt.Errorf("201 responses=%d, contracts in DB=%d, statuses=%v", created, n, statuses(rs))
		}
		return fmt.Sprintf("statuses=%v -> 1 contract", statuses(rs)), nil
	})
	if g.ContractID == "" {
		return g
	}

	s.r.Check("5 simultaneous pay taps return one PaymentIntent", func() (string, error) {
		rs := parallel(5, func(int) (int, map[string]any, error) {
			return s.createIntent(client.AccessToken, g.ProposalID, nil)
		})
		ids := map[string]bool{}
		var failed []string
		for _, r := range rs {
			if r.status != 200 {
				failed = append(failed, fmt.Sprintf("%d %s", r.status, errMsg(r.resp)))
				continue
			}
			g.PaymentIntentID = getStr(r.resp, "data", "payment_intent_id")
			ids[g.PaymentIntentID] = true
		}
		if len(ids) > 1 {
			return "", fmt.Errorf("%d different PaymentIntents created — a double tap can charge twice", len(ids))
		}
		if len(failed) > 0 {
			return "", fmt.Errorf("no double charge, but %d of 5 taps got an error the app would show: %s", len(failed), failed[0])
		}
		return "all 5 taps got " + g.PaymentIntentID, nil
	})
	s.r.MustCheck("G: pay (funds available immediately)", func() (string, error) {
		pi, err := s.confirmIntent(g.PaymentIntentID, pmBypassPending)
		if err != nil {
			return "", err
		}
		g.ChargeID = getStr(pi, "latest_charge")
		tx, err := s.waitForStatus(client.AccessToken, g.ContractID, "held", 30*time.Second)
		if err != nil {
			return "", err
		}
		g.TransactionID = getStr(tx, "id")
		return "held " + g.TransactionID, nil
	})
	fr.Ledger.hold(g.ContractID, g.Fees.FreelancerNet)
	client.Ledger.pay(g.TransactionID, g.Fees.ClientTotal)
	s.checkPaymentRow(g, "held")

	s.r.Check("simultaneous completes and releases pay out exactly once", func() (string, error) {
		if status, resp, err := s.complete(fr, g.ContractID); err != nil || status != 200 {
			return "", fmt.Errorf("freelancer complete: %d %v %s", status, err, errMsg(resp))
		}
		rs := parallel(8, func(i int) (int, map[string]any, error) {
			switch i % 3 {
			case 0:
				return s.complete(client, g.ContractID)
			case 1:
				return s.releaseContract(client, g.ContractID)
			default:
				return s.releaseContract(fr, g.ContractID)
			}
		})
		for _, r := range rs {
			if r.status >= 500 {
				return "", fmt.Errorf("a racing call crashed (%d): %s", r.status, errMsg(r.resp))
			}
		}
		time.Sleep(2 * time.Second)
		list, err := s.sc.Get("/transfers?limit=10&transfer_group="+g.ContractID, "")
		if err != nil {
			return "", err
		}
		if n := len(getRows(list, "data")); n != 1 {
			return "", fmt.Errorf("%d transfers for one escrow — double payout (statuses %v)", n, statuses(rs))
		}
		return fmt.Sprintf("statuses=%v -> 1 transfer", statuses(rs)), nil
	})
	fr.Ledger.release(g.ContractID)
	g.FinalStatus = "released"
	s.checkPaymentRow(g, "released")
	s.verifyTransfer(g)

	avail, err := s.availableToWithdraw(fr)
	if err != nil || avail <= 0 {
		s.r.Warn("withdrawal race", func() (string, error) {
			return "", fmt.Errorf("skipped: available_to_withdraw=%.2f (%v)", avail, err)
		})
		return g
	}
	s.r.Check("3 simultaneous full withdrawals can't take more than available", func() (string, error) {
		rs := parallel(3, func(int) (int, map[string]any, error) { return s.withdraw(fr, map[string]any{}) })
		var taken []float64
		for _, r := range rs {
			if r.status == 200 {
				taken = append(taken, getNum(r.resp, "data", "amount"))
			}
		}
		total := 0.0
		for _, t := range taken {
			total += t
			fr.Ledger.withdraw(t)
		}
		if len(taken) == 0 || round2(total) > avail+0.005 {
			return "", fmt.Errorf("available %.2f, withdrawn %v (statuses %v)", avail, taken, statuses(rs))
		}
		return fmt.Sprintf("available %.2f, withdrawn %v, statuses %v", avail, taken, statuses(rs)), nil
	})
	s.checkWallet(fr, "after withdrawal race")
	return g
}

func (s *Suite) concurrentRefundFlow(client, fr *TestUser) *Flow {
	s.r.Section("Deep — simultaneous admin refunds")
	h := s.setupFlow("H", client, fr, 5)
	s.payFlow(h, pmVisa)
	h.FinalStatus = "held"
	if !s.requireAdmin("simultaneous admin refunds") {
		return h
	}
	s.r.Check("3 simultaneous admin refunds refund exactly once", func() (string, error) {
		rs := parallel(3, func(int) (int, map[string]any, error) {
			return s.adminRefund(h.ContractID, "paymenttest: concurrent refund")
		})
		ok := 0
		for _, r := range rs {
			if r.status == 200 {
				ok++
			} else if r.status != 409 {
				return "", fmt.Errorf("unexpected status %d: %s", r.status, errMsg(r.resp))
			}
		}
		refunds, err := s.stripeRefunds(h.PaymentIntentID)
		if err != nil {
			return "", err
		}
		if ok != 1 || len(refunds) != 1 {
			return "", fmt.Errorf("%d successes, %d stripe refunds (statuses %v)", ok, len(refunds), statuses(rs))
		}
		return fmt.Sprintf("statuses=%v -> 1 refund", statuses(rs)), nil
	})
	fr.Ledger.dropHeld(h.ContractID)
	client.Ledger.refund(h.TransactionID)
	h.FinalStatus = "refunded"
	s.checkRefundRow(h, false)
	s.checkWallet(fr, "after concurrent refund")
	return h
}

func (s *Suite) dashboardRefundFlows(client, fr *TestUser) []*Flow {
	s.r.Section("Deep — refunds made directly in Stripe (Dashboard)")

	i := s.setupFlow("I", client, fr, 5)
	s.payFlow(i, pmVisa)
	s.r.Check("full Stripe refund marks the payment refunded (charge.refunded webhook)", func() (string, error) {
		if _, err := s.sc.Post("/refunds", url.Values{"payment_intent": {i.PaymentIntentID}}, ""); err != nil {
			return "", err
		}
		_, err := s.waitForStatus(client.AccessToken, i.ContractID, "refunded", 30*time.Second)
		return "refunded", err
	})
	if s.txStatus(i) == "refunded" {
		fr.Ledger.dropHeld(i.ContractID)
		client.Ledger.refund(i.TransactionID)
		i.FinalStatus = "refunded"
	} else {
		i.FinalStatus = "held"
	}
	s.r.Check("completing a Stripe-refunded contract releases nothing", func() (string, error) {
		st, err := s.completeBoth(i)
		if err != nil {
			return "", err
		}
		if st == "released" {
			return "", fmt.Errorf("refunded money was paid out to the freelancer")
		}
		return "payment_release=" + st, nil
	})
	s.noTransfer(i)
	s.checkWallet(fr, "after Stripe full refund")

	j := s.setupFlow("J", client, fr, 10)
	s.payFlow(j, pmVisa)
	j.FinalStatus = "held"
	s.r.Check("partial Stripe refund doesn't wipe out the whole payment", func() (string, error) {
		if _, err := s.sc.Post("/refunds", url.Values{"payment_intent": {j.PaymentIntentID}, "amount": {"100"}}, ""); err != nil {
			return "", err
		}
		time.Sleep(6 * time.Second) // charge.refunded webhook
		if st := s.txStatus(j); st != "held" {
			return "", fmt.Errorf("client got £1.00 back of £%.2f, but the whole payment is now %q — the freelancer can never be paid the £%.2f", j.Fees.ClientTotal, st, j.Fees.FreelancerNet)
		}
		return "still held after £1.00 partial refund", nil
	})
	if s.txStatus(j) == "refunded" {
		fr.Ledger.dropHeld(j.ContractID)
		client.Ledger.refund(j.TransactionID)
		j.FinalStatus = "refunded"
	}
	s.checkWallet(fr, "after Stripe partial refund")

	x := s.setupFlow("X", client, fr, 6)
	s.payFlow(x, pmBypassPending)
	s.r.MustCheck("X: job completed and the freelancer paid", func() (string, error) {
		st, err := s.completeBoth(x)
		if err != nil {
			return "", err
		}
		return st, expectEqual("payment_release", st, "released")
	})
	fr.Ledger.release(x.ContractID)
	s.verifyTransfer(x)
	s.r.Check("X: a full refund made in Stripe after release pulls the freelancer's transfer back", func() (string, error) {
		if _, err := s.sc.Post("/refunds", url.Values{"payment_intent": {x.PaymentIntentID}}, ""); err != nil {
			return "", err
		}
		if _, err := s.waitForStatus(client.AccessToken, x.ContractID, "refunded", 30*time.Second); err != nil {
			return "", err
		}
		t, err := s.sc.Get("/transfers/"+x.TransferID, "")
		if err != nil {
			return "", err
		}
		if !getBool(t, "reversed") || int64(getNum(t, "amount_reversed")) != pence(x.Fees.FreelancerNet) {
			return "", fmt.Errorf("transfer reversed=%v amount_reversed=%v", getBool(t, "reversed"), getNum(t, "amount_reversed"))
		}
		return fmt.Sprintf("%dp reversed", pence(x.Fees.FreelancerNet)), nil
	})
	fr.Ledger.dropReleased(x.ContractID)
	client.Ledger.refund(x.TransactionID)
	x.FinalStatus = "refunded"
	s.extraFlows = append(s.extraFlows, x)
	s.checkWallet(fr, "after a Stripe refund of a released payment")
	s.checkWallet(client, "after Stripe refunds")
	return []*Flow{i, j}
}

func (s *Suite) escrowStatus(contractID string) string {
	if s.db == nil {
		return ""
	}
	escs, err := s.db.escrows(contractID)
	if err != nil || len(escs) != 1 {
		return ""
	}
	return escs[0].Status
}

func (s *Suite) openChargeback(f *Flow) string {
	var disputeID string
	s.r.MustCheck(f.Label+": Stripe opens a chargeback on the payment", func() (string, error) {
		return pollUntil(60*time.Second, func() (string, bool, error) {
			list, err := s.sc.Get("/disputes?limit=5&payment_intent="+f.PaymentIntentID, "")
			if err != nil {
				return "", false, err
			}
			rows := getRows(list, "data")
			if len(rows) == 0 {
				return "no dispute yet", false, nil
			}
			disputeID = getStr(rows[0], "id")
			return fmt.Sprintf("%s status=%s", disputeID, getStr(rows[0], "status")), true, nil
		})
	})
	s.r.Check(f.Label+": the held payment is frozen while the bank disputes it", func() (string, error) {
		return pollUntil(30*time.Second, func() (string, bool, error) {
			st := s.escrowStatus(f.ContractID)
			return "escrow=" + st, st == "chargeback", nil
		})
	})
	return disputeID
}

func (s *Suite) closeChargeback(f *Flow, disputeID, evidence, want string) {
	s.r.MustCheck(fmt.Sprintf("%s: the chargeback closes as %s", f.Label, want), func() (string, error) {
		if _, err := s.sc.Post("/disputes/"+disputeID, url.Values{
			"evidence[uncategorized_text]": {evidence},
			"submit":                       {"true"},
		}, ""); err != nil {
			return "", err
		}
		return pollUntil(90*time.Second, func() (string, bool, error) {
			d, err := s.sc.Get("/disputes/"+disputeID, "")
			if err != nil {
				return "", false, err
			}
			return "status=" + getStr(d, "status"), getStr(d, "status") == want, nil
		})
	})
}

func (s *Suite) chargebackFlow(client, fr *TestUser) *Flow {
	s.r.Section("Deep — chargeback (card dispute)")
	k := s.setupFlow("K", client, fr, 5)
	k.CardDisputed = true
	s.payFlow(k, pmCreateDispute)
	k.FinalStatus = "held"
	disputeID := s.openChargeback(k)

	s.r.Check("K: completing the job while the bank disputes the payment releases nothing", func() (string, error) {
		st, err := s.completeBoth(k)
		if err != nil {
			return "", err
		}
		if st == "released" {
			fr.Ledger.release(k.ContractID)
			k.FinalStatus = "released"
			return "", fmt.Errorf("released £%.2f to the freelancer while the bank is clawing back £%.2f", k.Fees.FreelancerNet, k.Fees.ClientTotal)
		}
		return "payment_release=" + st, nil
	})
	if s.adminToken != "" {
		s.r.Check("K: an admin refund during the chargeback is refused with a clear reason", func() (string, error) {
			status, resp, err := s.adminRefund(k.ContractID, "trying to refund during a chargeback")
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 409); err != nil {
				return "", err
			}
			return errMsg(resp), nil
		})
	}
	s.checkWallet(fr, "during chargeback")

	s.closeChargeback(k, disputeID, "winning_evidence", "won")
	s.r.Check("K: a won chargeback puts the money back in escrow", func() (string, error) {
		return pollUntil(30*time.Second, func() (string, bool, error) {
			st := s.escrowStatus(k.ContractID)
			return "escrow=" + st, st == "held", nil
		})
	})
	s.r.Check("K: the completed job can now be paid out", func() (string, error) {
		status, resp, err := s.releaseContract(fr, k.ContractID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("released=%v", resp["released"]), expect(status, resp, 200)
	})
	if s.escrowStatus(k.ContractID) == "released" {
		fr.Ledger.release(k.ContractID)
		k.FinalStatus = "released"
	}
	s.checkWallet(fr, "after a won chargeback")

	k2 := s.setupFlow("K2", client, fr, 5)
	k2.CardDisputed = true
	s.payFlow(k2, pmCreateDispute)
	k2.FinalStatus = "held"
	d2 := s.openChargeback(k2)
	s.closeChargeback(k2, d2, "losing_evidence", "lost")
	s.r.Check("K2: a lost chargeback marks the payment refunded and never pays the freelancer", func() (string, error) {
		if _, err := pollUntil(30*time.Second, func() (string, bool, error) {
			st := s.escrowStatus(k2.ContractID)
			return "escrow=" + st, st == "refunded", nil
		}); err != nil {
			return "", err
		}
		st, err := s.completeBoth(k2)
		if err != nil {
			return "", err
		}
		if st == "released" {
			return "", fmt.Errorf("money the bank took back was released to the freelancer")
		}
		return "refunded; completion released nothing", nil
	})
	fr.Ledger.dropHeld(k2.ContractID)
	client.Ledger.refund(k2.TransactionID)
	k2.FinalStatus = "refunded"
	s.noTransfer(k2)
	s.checkWallet(fr, "after a lost chargeback")
	s.checkWallet(client, "after a lost chargeback")
	s.extraFlows = append(s.extraFlows, k2)
	return k
}

func (s *Suite) payBeforeAcceptFlows(client, fr *TestUser) *Flow {
	s.r.Section("Deep — client pays before accepting the proposal")
	l := s.proposalOnly("L", client, fr, 5, false)
	s.r.MustCheck("L: pay while the proposal is still pending", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, l.ProposalID, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		l.PaymentIntentID = getStr(resp, "data", "payment_intent_id")
		pi, err := s.confirmIntent(l.PaymentIntentID, pmVisa)
		if err != nil {
			return "", err
		}
		l.ChargeID = getStr(pi, "latest_charge")
		time.Sleep(4 * time.Second)
		return "paid " + l.PaymentIntentID, nil
	})
	s.r.MustCheck("L: accept, then create contract; payment is recorded", func() (string, error) {
		if status, resp, err := s.decide(client.AccessToken, l.ProposalID, "accepted"); err != nil || status != 200 {
			return "", fmt.Errorf("accept: %d %v %s", status, err, errMsg(resp))
		}
		status, resp, err := s.createContract(client.AccessToken, l.ProposalID, 5, time.Now(), time.Now().Add(24*time.Hour))
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 201); err != nil {
			return "", err
		}
		l.ContractID = getStr(resp, "contract", "id")
		tx, err := s.waitForStatus(client.AccessToken, l.ContractID, "held", 10*time.Second)
		if err != nil {
			return "", err
		}
		l.TransactionID = getStr(tx, "id")
		return "held " + l.TransactionID, nil
	})
	fr.Ledger.hold(l.ContractID, l.Fees.FreelancerNet)
	client.Ledger.pay(l.TransactionID, l.Fees.ClientTotal)
	l.FinalStatus = "held"
	s.checkPaymentRow(l, "held")

	payPending := func(f *Flow) (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, f.ProposalID, nil)
		if err != nil || status != 200 {
			return "", fmt.Errorf("intent: %d %v %s", status, err, errMsg(resp))
		}
		piID := getStr(resp, "data", "payment_intent_id")
		_, err = s.confirmIntent(piID, pmVisa)
		return piID, err
	}
	waitRefunded := func(piID string, f *Flow) (string, error) {
		return pollUntil(20*time.Second, func() (string, bool, error) {
			refunds, err := s.stripeRefunds(piID)
			if err != nil {
				return "", false, err
			}
			if len(refunds) == 1 && int64(getNum(refunds[0], "amount")) == pence(f.Fees.ClientTotal) {
				return "refunded " + getStr(refunds[0], "id"), true, nil
			}
			return fmt.Sprintf("%d refunds", len(refunds)), false, nil
		})
	}

	m := s.proposalOnly("M", client, fr, 5, false)
	s.r.Check("M: client pays, then rejects the proposal; the payment is refunded automatically", func() (string, error) {
		piID, err := payPending(m)
		if err != nil {
			return "", err
		}
		if status, resp, err := s.decide(client.AccessToken, m.ProposalID, "rejected"); err != nil || status != 200 {
			return "", fmt.Errorf("reject: %d %v %s", status, err, errMsg(resp))
		}
		return waitRefunded(piID, m)
	})
	s.r.Check("M: a rejected proposal can't be paid for again", func() (string, error) {
		status, resp, err := s.createIntent(client.AccessToken, m.ProposalID, nil)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})

	w := s.proposalOnly("W", client, fr, 5, false)
	s.r.Check("W: client pays, then the freelancer withdraws the proposal; the payment is refunded", func() (string, error) {
		piID, err := payPending(w)
		if err != nil {
			return "", err
		}
		status, resp, err := s.api.Do("POST", "/api/proposals/"+w.ProposalID+"/withdraw", fr.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		return waitRefunded(piID, w)
	})
	return l
}

func (s *Suite) findEvent(eventType, objectID string) (map[string]any, error) {
	list, err := s.sc.Get("/events?limit=100&type="+eventType, "")
	if err != nil {
		return nil, err
	}
	for _, ev := range getRows(list, "data") {
		if getStr(ev, "data", "object", "id") == objectID {
			return ev, nil
		}
	}
	return nil, fmt.Errorf("no %s event for %s", eventType, objectID)
}

func (s *Suite) replay(eventType, objectID string) (int, error) {
	ev, err := s.findEvent(eventType, objectID)
	if err != nil {
		return 0, err
	}
	raw, err := s.sc.GetRaw("/events/" + getStr(ev, "id"))
	if err != nil {
		return 0, err
	}
	ts := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write([]byte(ts + "."))
	mac.Write(raw)
	status, _, err := s.api.DoRaw("/api/v3/webhooks/stripe/payment", raw, map[string]string{
		"Content-Type":     "application/json",
		"Stripe-Signature": "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil)),
	})
	return status, err
}

func (s *Suite) webhookReplays(flows map[string]*Flow) {
	s.r.Section("Deep — Stripe webhook retries (signed replays)")
	if s.webhookSecret == "" {
		s.r.Skip("signed webhook replays", "no -webhook-secret")
		return
	}
	for _, c := range []struct {
		flow, eventType, why string
	}{
		{"A", "charge.succeeded", "a payment that was released"},
		{"E", "charge.succeeded", "a payment that was refunded"},
		{"F", "charge.succeeded", "a payment recorded at contract creation"},
		{"G", "charge.succeeded", "a payment raced by double taps"},
	} {
		c := c
		f := flows[c.flow]
		if f == nil || f.ChargeID == "" {
			continue
		}
		s.r.Check(fmt.Sprintf("replayed charge.succeeded for %s changes nothing (%s)", c.why, c.flow), func() (string, error) {
			before := s.txStatus(f)
			status, err := s.replay(c.eventType, f.ChargeID)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("webhook answered %d — Stripe would keep retrying", status)
			}
			rows, _, err := s.contractTransactions(f.Client.AccessToken, f.ContractID)
			if err != nil {
				return "", err
			}
			if len(rows) != 1 || getStr(rows[0], "status") != before {
				return "", fmt.Errorf("rows=%d status %s -> %s", len(rows), before, getStr(rows[0], "status"))
			}
			return fmt.Sprintf("200, still 1 row, status %s", before), nil
		})
	}
	s.r.Check("a charge that isn't a Tasksy payment is acknowledged (200), not retried forever", func() (string, error) {
		pi, err := s.sc.Post("/payment_intents", url.Values{
			"amount": {"150"}, "currency": {"gbp"}, "payment_method": {pmVisa}, "confirm": {"true"},
			"automatic_payment_methods[enabled]": {"true"}, "automatic_payment_methods[allow_redirects]": {"never"},
		}, "")
		if err != nil {
			return "", err
		}
		chargeID := getStr(pi, "latest_charge")
		var status int
		if _, err := pollUntil(30*time.Second, func() (string, bool, error) {
			st, err := s.replay("charge.succeeded", chargeID)
			if err != nil {
				return err.Error(), false, nil
			}
			status = st
			return fmt.Sprint(st), true, nil
		}); err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("webhook answered %d", status)
		}
		_, _ = s.sc.Post("/refunds", url.Values{"payment_intent": {getStr(pi, "id")}}, "")
		return "200", nil
	})
	if c := flows["C"]; c != nil && c.TransferID != "" {
		s.r.Check("replayed transfer.created doesn't revive a reversed+refunded escrow", func() (string, error) {
			status, err := s.replay("transfer.created", c.TransferID)
			if err != nil {
				return "", err
			}
			if st := s.txStatus(c); status != 200 || st != c.FinalStatus {
				return "", fmt.Errorf("webhook %d, status now %s (want %s)", status, st, c.FinalStatus)
			}
			return "200, still " + c.FinalStatus, nil
		})
	}
}
