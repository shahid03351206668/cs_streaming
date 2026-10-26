package paymentsv3

import (
	"strconv"

	"github.com/shopspring/decimal"
)

const PaymentCurrency = "gbp"

var hundred = decimal.NewFromInt(100)

// Client pays bid + client commission + platform fee; freelancer gets bid - commission - platform fee.
type FeeBreakdown struct {
	BidAmount               decimal.Decimal
	ClientCommissionPct     decimal.Decimal
	ClientCommission        decimal.Decimal
	ClientPlatformFee       decimal.Decimal
	ClientTotal             decimal.Decimal
	FreelancerCommissionPct decimal.Decimal
	FreelancerCommission    decimal.Decimal
	FreelancerPlatformFee   decimal.Decimal
	FreelancerNet           decimal.Decimal
	PlatformEarnings        decimal.Decimal
}

func CalculateFees(bid float64, s *SystemSetting) FeeBreakdown {
	b := decimal.NewFromFloat(bid).Round(2)
	clientPct := decimal.NewFromFloat(s.ClientCommission)
	freelancerPct := decimal.NewFromFloat(s.FreelancerCommission)
	platformFee := decimal.NewFromFloat(s.AppFee).Round(2)

	clientCommission := b.Mul(clientPct).Div(hundred).Round(2)
	freelancerCommission := b.Mul(freelancerPct).Div(hundred).Round(2)

	clientTotal := b.Add(clientCommission).Add(platformFee)
	freelancerNet := b.Sub(freelancerCommission).Sub(platformFee)

	return FeeBreakdown{
		BidAmount:               b,
		ClientCommissionPct:     clientPct,
		ClientCommission:        clientCommission,
		ClientPlatformFee:       platformFee,
		ClientTotal:             clientTotal,
		FreelancerCommissionPct: freelancerPct,
		FreelancerCommission:    freelancerCommission,
		FreelancerPlatformFee:   platformFee,
		FreelancerNet:           freelancerNet,
		PlatformEarnings:        clientTotal.Sub(freelancerNet),
	}
}

func ToPence(d decimal.Decimal) int64 {
	return d.Round(2).Mul(hundred).IntPart()
}

func fromPence(p int64) decimal.Decimal {
	return decimal.NewFromInt(p).Div(hundred)
}

// Stored on the PaymentIntent so the webhook records what was charged even if settings change later.
func (f FeeBreakdown) Metadata() map[string]string {
	p := func(d decimal.Decimal) string { return strconv.FormatInt(ToPence(d), 10) }
	return map[string]string{
		"fee_version":             "2",
		"bid_amount":              p(f.BidAmount),
		"client_commission":       p(f.ClientCommission),
		"client_platform_fee":     p(f.ClientPlatformFee),
		"client_total":            p(f.ClientTotal),
		"freelancer_commission":   p(f.FreelancerCommission),
		"freelancer_platform_fee": p(f.FreelancerPlatformFee),
		"freelancer_net":          p(f.FreelancerNet),
	}
}

// ok is false for intents created before fee_version 2.
func FeesFromMetadata(md map[string]string) (FeeBreakdown, bool) {
	if md["fee_version"] != "2" {
		return FeeBreakdown{}, false
	}
	keys := []string{"bid_amount", "client_commission", "client_platform_fee", "client_total",
		"freelancer_commission", "freelancer_platform_fee", "freelancer_net"}
	vals := make(map[string]decimal.Decimal, len(keys))
	for _, k := range keys {
		n, err := strconv.ParseInt(md[k], 10, 64)
		if err != nil {
			return FeeBreakdown{}, false
		}
		vals[k] = fromPence(n)
	}
	return FeeBreakdown{
		BidAmount:             vals["bid_amount"],
		ClientCommission:      vals["client_commission"],
		ClientPlatformFee:     vals["client_platform_fee"],
		ClientTotal:           vals["client_total"],
		FreelancerCommission:  vals["freelancer_commission"],
		FreelancerPlatformFee: vals["freelancer_platform_fee"],
		FreelancerNet:         vals["freelancer_net"],
		PlatformEarnings:      vals["client_total"].Sub(vals["freelancer_net"]),
	}, true
}

// decimal.Decimal marshals as a string, so amounts go out as float64.
func (f FeeBreakdown) Summary() map[string]any {
	n := func(d decimal.Decimal) float64 { return d.InexactFloat64() }
	return map[string]any{
		"currency":   PaymentCurrency,
		"bid_amount": n(f.BidAmount),
		"client": map[string]any{
			"bid_amount":            n(f.BidAmount),
			"commission_percentage": n(f.ClientCommissionPct),
			"commission":            n(f.ClientCommission),
			"platform_fee":          n(f.ClientPlatformFee),
			"total_to_pay":          n(f.ClientTotal),
		},
		"freelancer": map[string]any{
			"bid_amount":            n(f.BidAmount),
			"commission_percentage": n(f.FreelancerCommissionPct),
			"commission":            n(f.FreelancerCommission),
			"platform_fee":          n(f.FreelancerPlatformFee),
			"total_deducted":        n(f.FreelancerCommission.Add(f.FreelancerPlatformFee)),
			"amount_to_receive":     n(f.FreelancerNet),
		},
		"platform_earnings": n(f.PlatformEarnings),
	}
}
