package user

import (
	"strings"

	"tasksy/models"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/account"
)

// StripeConnectStatus is a plain-language summary of where a user's Stripe
// Connect account stands, for a client app to show directly to the user
// without needing to understand Stripe's requirements payload itself.
type StripeConnectStatus struct {
	Connected      bool     `json:"connected"`
	Status         string   `json:"status"` // not_connected | action_required | ready
	ChargesEnabled bool     `json:"charges_enabled"`
	PayoutsEnabled bool     `json:"payouts_enabled"`
	Missing        []string `json:"missing,omitempty"`
	Message        string   `json:"message"`
}

// requirementLabels maps Stripe's requirement keys to plain-language prompts.
// Unrecognized keys fall back to a readable version of the key itself, so a
// new requirement Stripe introduces still produces a sensible message rather
// than silently being dropped.
var requirementLabels = map[string]string{
	"external_account":                 "Add a bank account",
	"individual.dob.day":               "Add your date of birth",
	"individual.dob.month":             "Add your date of birth",
	"individual.dob.year":              "Add your date of birth",
	"individual.first_name":            "Add your legal first name",
	"individual.last_name":             "Add your legal last name",
	"individual.phone":                 "Add a valid phone number",
	"individual.email":                 "Add your email address",
	"individual.address.line1":         "Add your address",
	"individual.address.city":          "Add your address",
	"individual.address.postal_code":   "Add your address",
	"individual.address.state":         "Add your address",
	"individual.verification.document": "Upload an identity document",
}

func friendlyRequirement(key string) string {
	if label, ok := requirementLabels[key]; ok {
		return label
	}
	// individual.verification.additional_document -> "verification additional document"
	cleaned := strings.TrimPrefix(key, "individual.")
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	return "Please provide: " + cleaned
}

// dedupeFriendly collapses e.g. three separate dob.day/month/year entries
// down to one "Add your date of birth" message.
func dedupeFriendly(keys []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		label := friendlyRequirement(k)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

// GetStripeConnectStatus reports whether the user's Stripe Connect account
// exists, and if so, whether it's fully active or what's still needed.
func (s *Service) GetStripeConnectStatus(user *models.User) (*StripeConnectStatus, error) {
	if user.StripeConnectAccountID == "" {
		return &StripeConnectStatus{
			Connected: false,
			Status:    "not_connected",
			Message:   "Your payment account hasn't been set up yet. Please log out and log back in to set it up, then try again.",
		}, nil
	}

	stripe.Key = s.appConfig.Stripe.SecretKey
	acc, err := account.GetByID(user.StripeConnectAccountID, nil)
	if err != nil {
		return nil, err
	}

	result := &StripeConnectStatus{
		Connected:      true,
		ChargesEnabled: acc.ChargesEnabled,
		PayoutsEnabled: acc.PayoutsEnabled,
	}

	var due []string
	if acc.Requirements != nil {
		due = append(due, acc.Requirements.CurrentlyDue...)
	}

	if acc.ChargesEnabled && acc.PayoutsEnabled && len(due) == 0 {
		result.Status = "ready"
		result.Message = "Your payment account is fully set up."
		return result, nil
	}

	result.Status = "action_required"
	result.Missing = dedupeFriendly(due)
	if len(result.Missing) > 0 {
		result.Message = "A few things are needed before you can send or receive payments: " + strings.Join(result.Missing, ", ") + "."
	} else {
		// Requirements are empty but Stripe hasn't activated the account yet —
		// this is the transient "pending" verification window, not something
		// the user needs to act on.
		result.Message = "Your payment account is being verified. This can take a few minutes; please check back shortly."
	}
	return result, nil
}
