package chat

import (
	"fmt"
	"net/http"
	"strconv"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Handler struct {
	service Service
}

func (h *Handler) GetChatHistory(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation is missing", "message": "error"})
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err})
	}

	limit := 50
	messages, err := h.service.GetChatHistory(conversationID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch messages",
			"details": err.Error(),
		})
		return
	}
	// 5. Return Response
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    messages,
		"page":    page,
	})
}

func (h *Handler) GetInbox(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	conversations, err := h.service.GetInbox(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch inbox",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"count":   len(conversations),
		"data":    conversations,
	})
}
func NewHandler(s Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) InitiateChat(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var req struct {
		TargetID string `json:"target_id" binding:"required"`
		JobID    string `json:"job_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "target_id is required"})
		return
	}

	if req.TargetID == user.ID {
		c.JSON(400, gin.H{"error": "Cannot start a chat with yourself"})
		return
	}

	chat, err := h.service.InitiateChat(user.ID, req.TargetID, req.JobID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, chat)
}

func (h *Handler) SendMessage(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	convID := c.Param("id")

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request too large or invalid"})
		return
	}

	content := c.PostForm("content")
	form, _ := c.MultipartForm()
	files := form.File["files"]
	fmt.Println(form)
	fmt.Println(form.File)

	if content == "" && len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message cannot be empty. Provide text or a file."})
		return
	}

	msg, err := h.service.SendMessage(user.ID, convID, content, "text", files)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	c.JSON(http.StatusCreated, msg)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) WSHandler(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &Client{
		UserID:  userID,
		Conn:    conn,
		Send:    make(chan interface{}, 256),
		Service: h.service,
	}

	h.service.RegisterClient(client)

	go client.WritePump()
	go client.ReadPump()
}
