package chat

import (
	"fmt"
	"tasksy/models"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	CreateConversation(participants []string) (*models.ChatConversation, error)
	FindPrivateChat(userA, userB string) (*models.ChatConversation, error)
	GetUserConversations(userID string) ([]models.ChatConversation, error)
	SaveMessage(msg *models.ChatMessage) error
	GetHistory(conversationID string, limit, offset int) ([]models.ChatMessage, error)
	GetParticipantIDs(conversationID string) ([]string, error) // <--- Add this
	GetUserByID(userID string) (*models.User, error)
}

type chatRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &chatRepository{db: db}
}

func (r *chatRepository) GetUserByID(id string) (*models.User, error) {
	var user models.User
	err := r.db.Where("id = ?", id).First(&user).Error

	if err != nil {
		return nil, err
	}

	return &user, nil

}

func (r *chatRepository) GetParticipantIDs(conversationID string) ([]string, error) {
	var userIDs []string
	err := r.db.Model(&models.ChatParticipant{}).
		Where("conversation_id = ?", conversationID).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (r *chatRepository) GetHistory(conversationID string, limit, offset int) ([]models.ChatMessage, error) {
	var msgs []models.ChatMessage

	err := r.db.Where("conversation_id = ?", conversationID).
		Preload("Attachments").
		Preload("Sender").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&msgs).Error

	return msgs, err

}

func (r *chatRepository) CreateConversation(participants []string) (*models.ChatConversation, error) {
	tx := r.db.Begin()

	fmt.Println(participants)

	if len(participants) == 2 {
		var existingID string

		if err := r.db.Raw(`SELECT 
			p1.conversation_id 
            FROM chat_participants p1 
            JOIN chat_participants p2 ON p1.conversation_id = p2.conversation_id 
            WHERE p1.user_id = ? AND p2.user_id = ?
            LIMIT 1`,
			participants[0], participants[1]).Scan(&existingID).Error; err == nil && existingID != "" {
			var existingChat models.ChatConversation
			if err := r.db.First(&existingChat, "id = ?", existingID).Error; err == nil {
				return &existingChat, nil
			}
		}
	}

	chat := models.ChatConversation{LastSentAt: time.Now()}

	if err := tx.Create(&chat).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	for _, uid := range participants {
		err := tx.Create(&models.ChatParticipant{
			ConversationID: chat.ID, UserID: uid, JoinedAt: time.Now(),
		}).Error

		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	return &chat, tx.Commit().Error
}

func (r *chatRepository) FindPrivateChat(userA, userB string) (*models.ChatConversation, error) {
	var chat models.ChatConversation
	query := `
		SELECT c.* FROM chat_conversation c
		JOIN chat_participants p1 ON c.id = p1.conversation_id
		JOIN chat_participants p2 ON c.id = p2.conversation_id
		WHERE p1.user_id = ? AND p2.user_id = ? AND c.is_group = false
		LIMIT 1`

	if err := r.db.Raw(query, userA, userB).Scan(&chat).Error; err != nil {
		return nil, err
	}
	if chat.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &chat, nil
}

func (r *chatRepository) SaveMessage(msg *models.ChatMessage) error {
	tx := r.db.Begin()
	if err := tx.Create(msg).Error; err != nil {
		tx.Rollback()
		return err
	}
	// Update Conversation Last Message
	err := tx.Model(&models.ChatConversation{}).Where("id = ?", msg.ConversationID).
		Updates(map[string]interface{}{
			"last_message": msg.Content, "last_sent_at": time.Now(),
		}).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

//	func (r *chatRepository) GetHistory(conversationID string, limit, offset int) ([]models.ChatMessage, error) {
//		var msgs []models.ChatMessage
//		err := r.db.Where("conversation_id = ?", conversationID).
//			Preload("Attachments").Preload("Sender").
//			Order("created_at DESC").Limit(limit).Offset(offset).
//			Find(&msgs).Error
//		return msgs, err
//	}

func (r *chatRepository) GetUserConversations(userID string) ([]models.ChatConversation, error) {
	var chats []models.ChatConversation
	err := r.db.
		Table("chat_conversation"). // Use explicit table name
		Joins("JOIN chat_participants cp ON cp.conversation_id = chat_conversation.id").
		Where("cp.user_id = ?", userID).
		Preload("Participants").
		Preload("Participants.User").
		Order("last_sent_at DESC").
		Find(&chats).Error

	return chats, err
}
