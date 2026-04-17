package payments

import (
	"errors"
	"fmt"
	"tasksy/models"
	"time"

	"gorm.io/gorm"
)

// LedgerService handles all double-entry ledger operations.
type LedgerService struct {
	db *gorm.DB
}

func NewLedgerService(db *gorm.DB) *LedgerService {
	return &LedgerService{db: db}
}

// EntryInput represents a single leg to be created in a ledger transaction.
type EntryInput struct {
	AccountID string
	Amount    int64  // positive = credit, negative = debit
	Category  string // e.g. "escrow_deposit", "referral_reward", "platform_fee"
}

// CreateLedgerTransaction atomically creates a balanced transaction with its entries.
// It returns an error if the entries do not sum to zero (double-entry invariant).
func (s *LedgerService) CreateLedgerTransaction(
	txType models.LedgerTransactionType,
	referenceID string,
	description string,
	entries []EntryInput,
) (*models.LedgerTransaction, error) {

	if len(entries) < 2 {
		return nil, errors.New("ledger: a transaction requires at least two entries")
	}

	// ---- checksum: entries must balance to zero ----
	var sum int64
	for _, e := range entries {
		sum += e.Amount
	}
	if sum != 0 {
		return nil, fmt.Errorf("ledger: entries do not balance (sum = %d, expected 0)", sum)
	}

	txn := models.LedgerTransaction{
		Type:        txType,
		ReferenceID: referenceID,
		Status:      "posted",
		Description: description,
		PostingDate: time.Now(),
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&txn).Error; err != nil {
			return fmt.Errorf("ledger: failed to create transaction: %w", err)
		}

		glEntries := make([]models.GLEntry, 0, len(entries))
		for _, e := range entries {
			glEntries = append(glEntries, models.GLEntry{
				TransactionID: txn.ID,
				AccountID:     e.AccountID,
				Amount:        e.Amount,
				Category:      e.Category,
			})
		}

		if err := tx.Create(&glEntries).Error; err != nil {
			return fmt.Errorf("ledger: failed to create entries: %w", err)
		}

		txn.Entries = glEntries
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &txn, nil
}

// GetAccountBalance returns the current balance of an account using SUM(amount).
func (s *LedgerService) GetAccountBalance(accountID string) (int64, error) {
	var balance int64
	err := s.db.Model(&models.GLEntry{}).
		Where("account_id = ?", accountID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&balance).Error
	return balance, err
}

// GetAccountBalanceByType returns the aggregate balance for all accounts of a given type.
func (s *LedgerService) GetAccountBalanceByType(accountType models.AccountType) (int64, error) {
	var balance int64
	err := s.db.Model(&models.GLEntry{}).
		Joins("JOIN accounts ON accounts.id = gl_entries.account_id").
		Where("accounts.type = ?", accountType).
		Select("COALESCE(SUM(gl_entries.amount), 0)").
		Scan(&balance).Error
	return balance, err
}

// EnsureSystemAccounts creates the singleton system accounts if they don't exist.
// Call this once at application startup (e.g. after migrations).
func (s *LedgerService) EnsureSystemAccounts() error {
	systemAccounts := []struct {
		Type models.AccountType
		Name string
	}{
		{models.AccountTypeEscrow, "System Escrow"},
		{models.AccountTypeRevenue, "Platform Revenue"},
		{models.AccountTypeMarketing, "System Marketing"},
		{models.AccountTypeExternal, "External (Stripe/Bank)"},
	}

	for _, sa := range systemAccounts {
		var existing models.Account
		err := s.db.Where("type = ? AND user_id IS NULL", sa.Type).First(&existing).Error
		if err == nil {
			continue // already exists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("ledger: failed to check system account %s: %w", sa.Type, err)
		}
		acct := models.Account{Type: sa.Type, Name: sa.Name}
		if err := s.db.Create(&acct).Error; err != nil {
			return fmt.Errorf("ledger: failed to create system account %s: %w", sa.Type, err)
		}
	}
	return nil
}

// GetSystemAccount returns the system account for a given type.
func (s *LedgerService) GetSystemAccount(accountType models.AccountType) (*models.Account, error) {
	var acct models.Account
	err := s.db.Where("type = ? AND user_id IS NULL", accountType).First(&acct).Error
	if err != nil {
		return nil, fmt.Errorf("ledger: system account %s not found: %w", accountType, err)
	}
	return &acct, nil
}

func (s *LedgerService) GetOrCreateUserAccount(userID string) (*models.Account, error) {
	var acct models.Account
	err := s.db.Where("type = ? AND user_id = ?", models.AccountTypeUserWallet, userID).First(&acct).Error
	if err == nil {
		return &acct, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Verify user exists
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("ledger: user %s not found", userID)
	}

	acct = models.Account{
		UserID: &userID,
		Type:   models.AccountTypeUserWallet,
		Name:   fmt.Sprintf("Wallet – %s %s", user.FirstName, user.LastName),
	}
	if err := s.db.Create(&acct).Error; err != nil {
		return nil, err
	}
	return &acct, nil
}

func (s *LedgerService) GetUserWalletBalance(userID string) (int64, error) {
	var balance int64
	err := s.db.Model(&models.GLEntry{}).
		Joins("JOIN accounts ON accounts.id = gl_entries.account_id").
		Where("accounts.type = ? AND accounts.user_id = ?", models.AccountTypeUserWallet, userID).
		Select("COALESCE(SUM(gl_entries.amount), 0)").
		Scan(&balance).Error
	return balance, err
}

// SystemIntegrityCheck returns the sum of ALL gl_entries. Must be 0 for a healthy system.
func (s *LedgerService) SystemIntegrityCheck() (int64, error) {
	var sum int64
	err := s.db.Model(&models.GLEntry{}).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&sum).Error
	return sum, err
}
