package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *Suite) fileDispute(f *Flow) (string, error) {
	status, resp, err := s.api.PostForm("/api/v1/disputes", f.Freelancer.AccessToken, url.Values{
		"contract_id": {f.ContractID},
		"reason":      {"Disagreement over the work"},
		"description": {"Dispute filed by the paymenttest suite on " + f.Label},
	}, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200, 201); err != nil {
		return "", err
	}
	return getStr(resp, "data", "id"), nil
}

func (s *Suite) resolveDispute(id string, body map[string]any) (int, map[string]any, error) {
	if _, ok := body["resolution"]; !ok {
		body["resolution"] = "paymenttest: admin decision"
	}
	return s.api.Do("PUT", "/api/v1/admin/disputes/"+id+"/resolve", s.adminToken, body, nil)
}

func (s *Suite) contractStatus(token, contractID string) (string, error) {
	status, resp, err := s.api.Do("GET", "/api/contracts/list?contract_id="+contractID, token, nil, nil)
	if err != nil {
		return "", err
	}
	if err := expect(status, resp, 200); err != nil {
		return "", err
	}
	rows := getRows(resp, "data")
	if len(rows) != 1 {
		return "", fmt.Errorf("%d contracts returned", len(rows))
	}
	return getStr(rows[0], "status"), nil
}

func (s *Suite) disputedFlow(label string, client, fr *TestUser, bid float64, pm string) (*Flow, string) {
	f := s.setupFlow(label, client, fr, bid)
	s.payFlow(f, pm)
	var id string
	s.r.MustCheck(label+": freelancer files a dispute", func() (string, error) {
		var err error
		id, err = s.fileDispute(f)
		return id, err
	})
	return f, id
}

func (s *Suite) disputeOutcomeFlows(client, fr *TestUser) []*Flow {
	s.r.Section("Dispute resolution — who wins, penalties and fee reversal")
	if !s.requireAdmin("dispute outcomes") {
		return nil
	}

	n, nID := s.disputedFlow("N", client, fr, 10, pmBypassPending)
	s.r.Check("invalid outcome is refused and nothing moves", func() (string, error) {
		status, resp, err := s.resolveDispute(nID, map[string]any{"outcome": "everyone"})
		if err != nil {
			return "", err
		}
		msg, err := expectRefused(status, resp, "outcome")
		if err != nil {
			return "", err
		}
		if st := s.txStatus(n); st != "held" {
			return "", fmt.Errorf("payment is %s after a refused resolve", st)
		}
		return msg, nil
	})
	s.r.Check("short remarks are refused", func() (string, error) {
		status, resp, err := s.api.Do("PUT", "/api/v1/admin/disputes/"+nID+"/resolve", s.adminToken, map[string]any{"resolution": "ok", "outcome": "client"}, nil)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "")
	})
	s.r.Check("non-admin cannot resolve a dispute (403) and nothing moves", func() (string, error) {
		for _, u := range []*TestUser{client, fr} {
			status, resp, err := s.api.Do("PUT", "/api/v1/admin/disputes/"+nID+"/resolve", u.AccessToken, map[string]any{
				"resolution": "trying to decide my own dispute", "outcome": "freelancer",
			}, nil)
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 403); err != nil {
				return "", fmt.Errorf("%s: %w", u.Label, err)
			}
		}
		if st := s.txStatus(n); st != "held" {
			return "", fmt.Errorf("payment is %s", st)
		}
		return "403 for both parties, payment still held", nil
	})

	s.r.MustCheck("N: admin rules for the freelancer; escrow is paid out", func() (string, error) {
		status, resp, err := s.resolveDispute(nID, map[string]any{"outcome": "freelancer"})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		d := getMap(resp, "data")
		if getStr(d, "status") != "resolved" || getStr(d, "outcome") != "freelancer" || getStr(d, "payout_action") != "released_to_freelancer" {
			return "", fmt.Errorf("dispute=%s outcome=%s payout=%s", getStr(d, "status"), getStr(d, "outcome"), getStr(d, "payout_action"))
		}
		return "resolved, payout_action=released_to_freelancer", nil
	})
	fr.Ledger.release(n.ContractID)
	n.FinalStatus = "released"
	s.checkPaymentRow(n, "released")
	s.verifyTransfer(n)
	s.r.Check("N: contract is completed", func() (string, error) {
		st, err := s.contractStatus(client.AccessToken, n.ContractID)
		if err != nil {
			return "", err
		}
		return st, expectEqual("contract status", st, "completed")
	})
	s.r.Check("N: resolving the same dispute again is refused", func() (string, error) {
		status, resp, err := s.resolveDispute(nID, map[string]any{"outcome": "client"})
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "already resolved")
	})
	s.checkWallet(fr, "after dispute won by freelancer")

	o, oID := s.disputedFlow("O", client, fr, 10, pmVisa)
	s.r.MustCheck("O: admin rules for the client with fees reversed; client is refunded in full", func() (string, error) {
		status, resp, err := s.resolveDispute(oID, map[string]any{"outcome": "client"})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if pa := getStr(resp, "data", "payout_action"); pa != "refunded_in_full" {
			return "", fmt.Errorf("payout_action=%s", pa)
		}
		return "payout_action=refunded_in_full", nil
	})
	fr.Ledger.dropHeld(o.ContractID)
	client.Ledger.refund(o.TransactionID)
	o.FinalStatus = "refunded"
	s.checkStripeRefund(o)
	s.checkRefundRow(o, false)
	s.noTransfer(o)
	s.r.Check("O: contract is cancelled", func() (string, error) {
		st, err := s.contractStatus(client.AccessToken, o.ContractID)
		if err != nil {
			return "", err
		}
		return st, expectEqual("contract status", st, "cancelled")
	})
	s.r.Check("O: completing the cancelled contract is refused", func() (string, error) {
		status, resp, err := s.complete(fr, o.ContractID)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "cancelled")
	})
	s.checkWallet(fr, "after dispute won by client")
	s.checkWallet(client, "after dispute won by client")

	p, pID := s.disputedFlow("P", client, fr, 10, pmVisa)
	before, statsErr := s.adminStats()
	s.r.MustCheck("P: admin rules for the client and keeps the fees as a penalty", func() (string, error) {
		status, resp, err := s.resolveDispute(pID, map[string]any{"outcome": "client", "refund_fees": false})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if pa := getStr(resp, "data", "payout_action"); pa != "refunded_excluding_fees" {
			return "", fmt.Errorf("payout_action=%s", pa)
		}
		return "payout_action=refunded_excluding_fees", nil
	})
	fr.Ledger.dropHeld(p.ContractID)
	client.Ledger.refund(p.TransactionID)
	client.Ledger.keep(p.TransactionID, round2(p.Fees.ClientTotal-p.Fees.Bid))
	p.FinalStatus = "refunded"
	s.r.Check("P: client got the bid back; the commission and platform fee were kept", func() (string, error) {
		refunds, err := s.stripeRefunds(p.PaymentIntentID)
		if err != nil {
			return "", err
		}
		if len(refunds) != 1 {
			return "", fmt.Errorf("%d refunds", len(refunds))
		}
		want := pence(p.Fees.Bid)
		if got := int64(getNum(refunds[0], "amount")); got != want {
			return "", fmt.Errorf("refunded %dp, expected the bid %dp (paid %dp)", got, want, pence(p.Fees.ClientTotal))
		}
		if s.db != nil {
			pays, err := s.db.payments(p.ContractID)
			if err != nil || len(pays) != 1 {
				return "", fmt.Errorf("payments=%d err=%v", len(pays), err)
			}
			if !eq(pays[0].RefundedAmount, p.Fees.Bid) || pays[0].Status != "refunded" {
				return "", fmt.Errorf("db refunded_amount=%.2f status=%s", pays[0].RefundedAmount, pays[0].Status)
			}
		}
		return fmt.Sprintf("refunded £%.2f of £%.2f, platform kept £%.2f", p.Fees.Bid, p.Fees.ClientTotal, round2(p.Fees.ClientTotal-p.Fees.Bid)), nil
	})
	s.noTransfer(p)
	s.r.Check("P: admin stats keep the retained fees but drop freelancer net", func() (string, error) {
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
		}{{"total_volume", p.Fees.Bid}, {"total_fees", p.Fees.FreelancerFee}, {"total_net", p.Fees.FreelancerNet}} {
			if drop := round2(getNum(before, c.key) - getNum(after, c.key)); !eq(drop, c.want) {
				problems = append(problems, fmt.Sprintf("%s dropped %.2f want %.2f", c.key, drop, c.want))
			}
		}
		if len(problems) > 0 {
			return "", fmt.Errorf("%s", strings.Join(problems, "; "))
		}
		return "volume -bid, fees -freelancer side, net -freelancer net", nil
	})
	s.checkWallet(fr, "after penalty refund")
	s.checkWallet(client, "after penalty refund")

	q, qID := s.disputedFlow("Q", client, fr, 6, pmVisa)
	s.r.Check("Q: client refunded in Stripe before the admin rules for the client", func() (string, error) {
		if _, err := s.sc.Post("/refunds", url.Values{"payment_intent": {q.PaymentIntentID}}, ""); err != nil {
			return "", err
		}
		_, err := s.waitForStatus(client.AccessToken, q.ContractID, "refunded", 30*time.Second)
		return "refunded", err
	})
	s.r.Check("Q: resolving does not refund twice", func() (string, error) {
		status, resp, err := s.resolveDispute(qID, map[string]any{"outcome": "client"})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if pa := getStr(resp, "data", "payout_action"); pa != "already_refunded" {
			return "", fmt.Errorf("payout_action=%s", pa)
		}
		refunds, err := s.stripeRefunds(q.PaymentIntentID)
		if err != nil || len(refunds) != 1 {
			return "", fmt.Errorf("stripe refunds=%d err=%v", len(refunds), err)
		}
		return "already_refunded, still 1 refund", nil
	})
	fr.Ledger.dropHeld(q.ContractID)
	client.Ledger.refund(q.TransactionID)
	q.FinalStatus = "refunded"

	r := s.setupFlow("R", client, fr, 6)
	rID, derr := s.fileDispute(r)
	s.r.Check("R: dispute on an unpaid contract resolves with nothing to move", func() (string, error) {
		if derr != nil {
			return "", derr
		}
		status, resp, err := s.resolveDispute(rID, map[string]any{"outcome": "client"})
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if pa := getStr(resp, "data", "payout_action"); pa != "none" {
			return "", fmt.Errorf("payout_action=%s", pa)
		}
		return "payout_action=none", nil
	})

	return []*Flow{n, o, p, q}
}

func expectEqual(what, got, want string) error {
	if got != want {
		return fmt.Errorf("%s is %q, want %q", what, got, want)
	}
	return nil
}
