package main

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB gives the suite direct read access to payment state, so checks can
// assert invariants the API doesn't expose (one payment row per contract,
// exact stored amounts, no escrow stuck mid-transition). Its only write is
// granting the admin role to the suite's own admin test user.
type DB struct{ g *gorm.DB }

func openDB(dsn string) (*DB, error) {
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	return &DB{g: g}, nil
}

func (d *DB) grantAdmin(userID string) error {
	if err := d.g.Exec(`INSERT INTO roles (name) VALUES ('admin') ON CONFLICT DO NOTHING`).Error; err != nil {
		return err
	}
	return d.g.Exec(`INSERT INTO user_roles (user_id, role_name) VALUES (?, 'admin') ON CONFLICT DO NOTHING`, userID).Error
}

type paymentRow struct {
	ID                    string
	Status                string
	StripePaymentIntentID string
	StripeChargeID        string
	StripeRefundID        string
	StripeReversalID      string
	RefundedByAdminID     string
	RefundReason          string
	GrossAmount           float64
	ClientFee             float64
	FreelancerFee         float64
	PlatformFee           float64
	NetAmount             float64
}

type escrowRow struct {
	ID                   string
	PaymentTransactionID string
	Status               string
	StripeTransferID     string
	Amount               float64
}

func (d *DB) payments(contractID string) ([]paymentRow, error) {
	var rows []paymentRow
	err := d.g.Raw(`SELECT id, status, stripe_payment_intent_id, stripe_charge_id,
			COALESCE(stripe_refund_id, '') AS stripe_refund_id,
			COALESCE(stripe_reversal_id, '') AS stripe_reversal_id,
			COALESCE(refunded_by_admin_id, '') AS refunded_by_admin_id,
			COALESCE(refund_reason, '') AS refund_reason,
			gross_amount::float8 AS gross_amount, client_fee::float8 AS client_fee,
			freelancer_fee::float8 AS freelancer_fee, platform_fee::float8 AS platform_fee,
			net_amount::float8 AS net_amount
		FROM payment_transactions_v3 WHERE contract_id = ? AND deleted_at IS NULL ORDER BY created_at`, contractID).
		Scan(&rows).Error
	return rows, err
}

func (d *DB) escrows(contractID string) ([]escrowRow, error) {
	var rows []escrowRow
	err := d.g.Raw(`SELECT id, payment_transaction_id, status,
			COALESCE(stripe_transfer_id, '') AS stripe_transfer_id, amount::float8 AS amount
		FROM escrow_transactions_v3 WHERE contract_id = ? AND deleted_at IS NULL ORDER BY created_at`, contractID).
		Scan(&rows).Error
	return rows, err
}
