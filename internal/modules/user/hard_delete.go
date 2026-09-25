package user

import (
	"fmt"

	"tasksy/models"

	"gorm.io/gorm"
)

// HardDeleteAccount permanently removes a user and every row across the
// schema that references them. Every model embeds BaseModel's soft-delete
// (deleted_at), so a plain Delete() would only hide rows — Unscoped() is
// required throughout to actually purge them. This is irreversible.
//
// Known limitation: uploaded files (S3 objects for profile photo, job media,
// portfolio, certifications, chat/proposal attachments) are not removed from
// storage — only their DB rows are. The user's Stripe Connect account (if
// any) is also left untouched. Both would need separate cleanup.
func (s *Service) HardDeleteAccount(userID string) error {
	var exists models.User
	if err := s.db.First(&exists, "id = ?", userID).Error; err != nil {
		return ErrUserNotFound
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var jobPostIDs []string
		tx.Model(&models.JobPost{}).Where("created_by_id = ?", userID).Pluck("id", &jobPostIDs)

		var proposalIDs []string
		tx.Model(&models.Proposal{}).
			Where("freelancer_id = ? OR job_post_id IN ?", userID, orNilList(jobPostIDs)).
			Pluck("id", &proposalIDs)

		var contractIDs []string
		tx.Model(&models.Contract{}).
			Where("client_id = ? OR freelancer_id = ? OR job_post_id IN ?", userID, userID, orNilList(jobPostIDs)).
			Pluck("id", &contractIDs)

		var referralCodeIDs []string
		tx.Model(&models.ReferralCode{}).Where("owner_id = ?", userID).Pluck("id", &referralCodeIDs)

		var notifPrefIDs []string
		tx.Model(&models.NotificationPreference{}).Where("user_id = ?", userID).Pluck("id", &notifPrefIDs)

		var portfolioIDs []string
		tx.Model(&models.Portfolio{}).Where("user_id = ?", userID).Pluck("id", &portfolioIDs)

		var messageIDs []string
		tx.Model(&models.ChatMessage{}).Where("sender_id = ?", userID).Pluck("id", &messageIDs)

		// Reviews about to be deleted may leave someone else's target_id rating
		// stale — collect those recipients now so we can recompute after commit.
		var affectedRatingTargets []string
		tx.Model(&models.Review{}).
			Where("(reviewer_id = ? OR target_id = ? OR contract_id IN ?) AND target_id <> ?",
				userID, userID, orNilList(contractIDs), userID).
			Distinct("target_id").
			Pluck("target_id", &affectedRatingTargets)

		steps := []func() error{
			// Proposal attachments, then proposals on this user's own jobs / submitted by them.
			func() error {
				return tx.Unscoped().Where("proposal_id IN ?", orNilList(proposalIDs)).
					Delete(&models.ProposalAttachment{}).Error
			},
			func() error {
				return tx.Unscoped().Where("job_id IN ?", orNilList(jobPostIDs)).Delete(&models.JobMedia{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("reporter_id = ? OR job_post_id IN ?", userID, orNilList(jobPostIDs)).
					Delete(&models.JobReport{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("reviewer_id = ? OR target_id = ? OR contract_id IN ?", userID, userID, orNilList(contractIDs)).
					Delete(&models.Review{}).Error
			},
			// Preserve disputes belonging to other users' contracts; just detach this user as resolver.
			func() error {
				return tx.Unscoped().Model(&models.Dispute{}).
					Where("resolved_by_id = ?", userID).
					Update("resolved_by_id", nil).Error
			},
			func() error {
				return tx.Unscoped().
					Where("filed_by_id = ? OR contract_id IN ?", userID, orNilList(contractIDs)).
					Delete(&models.Dispute{}).Error
			},
			// Financial records mentioning this user — removed as requested; see doc comment.
			func() error {
				var txnIDs []string
				tx.Model(&models.PaymentTransactionV3{}).
					Where("from_user_id = ? OR to_user_id = ?", userID, userID).
					Pluck("id", &txnIDs)
				return tx.Unscoped().
					Where("user_id = ? OR payment_transaction_id IN ?", userID, orNilList(txnIDs)).
					Delete(&models.EscrowTransactionV3{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("from_user_id = ? OR to_user_id = ?", userID, userID).
					Delete(&models.PaymentTransactionV3{}).Error
			},
			func() error {
				return tx.Unscoped().Where("id IN ?", orNilList(contractIDs)).Delete(&models.Contract{}).Error
			},
			func() error {
				return tx.Unscoped().Where("id IN ?", orNilList(proposalIDs)).Delete(&models.Proposal{}).Error
			},
			func() error {
				return tx.Unscoped().Where("id IN ?", orNilList(jobPostIDs)).Delete(&models.JobPost{}).Error
			},
			// Chat: remove this user's own messages/attachments and their participant
			// record, but leave the conversation shell (and the other party's history) intact.
			func() error {
				return tx.Unscoped().Where("message_id IN ?", orNilList(messageIDs)).
					Delete(&models.ChatAttachment{}).Error
			},
			func() error {
				return tx.Unscoped().Where("id IN ?", orNilList(messageIDs)).Delete(&models.ChatMessage{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ChatParticipant{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("blocker_id = ? OR blocked_id = ?", userID, userID).
					Delete(&models.UserBlock{}).Error
			},
			func() error {
				return tx.Unscoped().Where("preference_id IN ?", orNilList(notifPrefIDs)).
					Delete(&models.NotificationPreferenceCategory{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.NotificationPreference{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Notification{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.DeviceToken{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.UserBankAccount{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.UserAddress{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("entity_type = ? AND entity_id IN ?", "portfolio", orNilList(portfolioIDs)).
					Delete(&models.File{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Portfolio{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Certification{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.UserFeedPreferences{}).Error
			},
			func() error {
				return tx.Unscoped().
					Where("referral_code_id IN ? OR referrer_id = ? OR referee_id = ?", orNilList(referralCodeIDs), userID, userID).
					Delete(&models.ReferralUsage{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_id = ?", userID).Delete(&models.ReferralCode{}).Error
			},
			func() error {
				return tx.Exec("DELETE FROM user_roles WHERE user_id = ?", userID).Error
			},
			func() error {
				return tx.Unscoped().Where("id = ?", userID).Delete(&models.User{}).Error
			},
		}

		for i, step := range steps {
			if err := step(); err != nil {
				return fmt.Errorf("hard delete step %d failed: %w", i, err)
			}
		}

		for _, targetID := range affectedRatingTargets {
			recomputeRatingAggregate(tx, targetID)
		}
		return nil
	})
}

// recomputeRatingAggregate recalculates a user's denormalized rating and
// reviews_count from the reviews table. Shared by review creation and by
// hard-delete, which can remove reviews that belonged to other users.
func recomputeRatingAggregate(db *gorm.DB, userID string) {
	var agg struct {
		Avg   float64
		Count int64
	}
	if err := db.Model(&models.Review{}).
		Where("target_id = ?", userID).
		Select("COALESCE(AVG(rating), 0) as avg, COUNT(*) as count").
		Scan(&agg).Error; err != nil {
		return
	}

	db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{
		"rating":        agg.Avg,
		"reviews_count": agg.Count,
	})
}

// orNilList keeps GORM's `IN ?` clause well-formed for an empty slice.
func orNilList(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
