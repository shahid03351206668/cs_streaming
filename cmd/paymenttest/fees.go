package main

import "math"

type SystemSettings struct {
	ClientCommissionPct     float64
	FreelancerCommissionPct float64
	PlatformFee             float64
}

func parseSettings(m map[string]any) SystemSettings {
	return SystemSettings{
		ClientCommissionPct:     getNum(m, "client_commission_percentage"),
		FreelancerCommissionPct: getNum(m, "freelancer_commission_percentage"),
		PlatformFee:             getNum(m, "application_fee_amount"),
	}
}

type ExpectedFees struct {
	Bid              float64
	ClientFee        float64 // client commission + platform fee
	FreelancerFee    float64 // freelancer commission + platform fee
	ClientTotal      float64
	FreelancerNet    float64
	PlatformEarnings float64
}

// computeFees re-derives the server's fee formula independently, so the
// suite verifies the server's arithmetic instead of echoing it back.
//
//	client pays:     bid + client commission + platform fee
//	freelancer gets: bid - freelancer commission - platform fee
func computeFees(bid float64, s SystemSettings) ExpectedFees {
	clientCommission := round2(bid * s.ClientCommissionPct / 100)
	freelancerCommission := round2(bid * s.FreelancerCommissionPct / 100)
	platformFee := round2(s.PlatformFee)
	clientTotal := round2(bid + clientCommission + platformFee)
	freelancerNet := round2(bid - freelancerCommission - platformFee)
	return ExpectedFees{
		Bid:              bid,
		ClientFee:        round2(clientCommission + platformFee),
		FreelancerFee:    round2(freelancerCommission + platformFee),
		ClientTotal:      clientTotal,
		FreelancerNet:    freelancerNet,
		PlatformEarnings: round2(clientTotal - freelancerNet),
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func pence(v float64) int64 { return int64(math.Round(v * 100)) }

func eq(a, b float64) bool { return math.Abs(a-b) < 0.005 }
