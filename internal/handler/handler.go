package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"

	"robertomachorro/smartchat/internal/config"
	"robertomachorro/smartchat/internal/service/auth"
	"robertomachorro/smartchat/internal/service/chat"
)

const (
	sessionUserEmail     = "user_email"
	sessionChatID        = "chat_id"
	sessionOAuthState    = "oauth_state"
	sessionOAuthProvider = "oauth_provider"
	sessionModel         = "model"
	sessionTemperature   = "temperature"
)

type Handler struct {
	Config   config.Config
	Sessions *sessions.CookieStore
	Auth     *auth.Service
	Chat     *chat.Service
	Now      func() time.Time
}

func NewHandler(cfg config.Config, store *sessions.CookieStore, authSvc *auth.Service, chatSvc *chat.Service) *Handler {
	return &Handler{Config: cfg, Sessions: store, Auth: authSvc, Chat: chatSvc, Now: time.Now}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/login", h.ShowLogin)
	router.GET("/auth/google", h.StartOAuth(auth.ProviderGoogle))
	router.GET("/auth/google/callback", h.HandleOAuthCallback(auth.ProviderGoogle))
	router.GET("/auth/github", h.StartOAuth(auth.ProviderGitHub))
	router.GET("/auth/github/callback", h.HandleOAuthCallback(auth.ProviderGitHub))
	router.GET("/logout", h.Logout)

	authed := router.Group("/")
	authed.Use(h.RequireAuth)
	authed.GET("/", h.ShowChat)
	authed.GET("/chat/:id", h.ShowChat)
	authed.POST("/chat/:id/message", h.PostMessage)
	authed.POST("/api/chat/:id/message", h.PostMessage)
	authed.POST("/api/preferences", h.UpdatePreferences)
}

func (h *Handler) RequireAuth(c *gin.Context) {
	session := h.session(c)
	if session == nil {
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
		return
	}
	if session.Values[sessionUserEmail] == nil {
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
		return
	}
	c.Next()
}

func (h *Handler) ShowLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"InstanceName": h.Config.InstanceName,
	})
}

func (h *Handler) StartOAuth(provider auth.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := h.session(c)
		if session == nil {
			c.String(http.StatusInternalServerError, "session unavailable")
			return
		}
		state := randomState()
		session.Values[sessionOAuthState] = state
		session.Values[sessionOAuthProvider] = string(provider)
		if err := session.Save(c.Request, c.Writer); err != nil {
			c.String(http.StatusInternalServerError, "session save failed")
			return
		}
		url, err := h.Auth.AuthURL(provider, state)
		if err != nil {
			c.String(http.StatusInternalServerError, "oauth config failed")
			return
		}
		c.Redirect(http.StatusFound, url)
	}
}

func (h *Handler) HandleOAuthCallback(provider auth.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := h.session(c)
		if session == nil {
			c.String(http.StatusInternalServerError, "session unavailable")
			return
		}
		state := c.Query("state")
		code := c.Query("code")
		if state == "" || code == "" {
			c.String(http.StatusBadRequest, "missing oauth state or code")
			return
		}
		storedState, _ := session.Values[sessionOAuthState].(string)
		storedProvider, _ := session.Values[sessionOAuthProvider].(string)
		if storedState == "" || storedProvider == "" || storedState != state || storedProvider != string(provider) {
			c.String(http.StatusBadRequest, "invalid oauth state")
			return
		}
		token, err := h.Auth.Exchange(c.Request.Context(), provider, code)
		if err != nil {
			c.String(http.StatusBadRequest, "oauth exchange failed")
			return
		}
		email, err := h.Auth.FetchEmail(c.Request.Context(), provider, token)
		if err != nil {
			c.String(http.StatusBadRequest, "failed to fetch email")
			return
		}
		session.Values[sessionUserEmail] = email
		session.Values[sessionOAuthState] = ""
		session.Values[sessionOAuthProvider] = ""
		if session.Values[sessionTemperature] == nil {
			session.Values[sessionTemperature] = 0.5
		}
		if session.Values[sessionModel] == nil && len(h.Config.OpenAI.Models) > 0 {
			session.Values[sessionModel] = h.Config.OpenAI.Models[0]
		}
		if err := session.Save(c.Request, c.Writer); err != nil {
			c.String(http.StatusInternalServerError, "session save failed")
			return
		}
		c.Redirect(http.StatusFound, "/")
	}
}

func (h *Handler) Logout(c *gin.Context) {
	session := h.session(c)
	if session != nil {
		session.Options.MaxAge = -1
		_ = session.Save(c.Request, c.Writer)
	}
	c.Redirect(http.StatusFound, "/login")
}

func (h *Handler) ShowChat(c *gin.Context) {
	userEmail := h.userEmail(c)
	chatID := c.Param("id")
	if chatID == "" {
		currentChat, _ := h.getSessionChatID(c)
		chatID = currentChat
	}
	if chatID == "" {
		summary, err := h.Chat.EnsureChat(c.Request.Context(), userEmail)
		if err != nil {
			c.String(http.StatusInternalServerError, "chat setup failed")
			return
		}
		chatID = summary.ID
		_ = h.setSessionChatID(c, chatID)
	}
	if c.Param("id") != "" {
		_ = h.setSessionChatID(c, chatID)
	}
	view, err := h.Chat.GetChat(c.Request.Context(), userEmail, chatID)
	if err != nil {
		c.String(http.StatusBadRequest, "chat not found")
		return
	}
	chats, err := h.Chat.ListChats(c.Request.Context(), userEmail)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load chats")
		return
	}
	model, temperature := h.sessionPreferences(c)
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"InstanceName": h.Config.InstanceName,
		"UserEmail":    userEmail,
		"Chat":         view,
		"Chats":        chats,
		"Models":       h.Config.OpenAI.Models,
		"Model":        model,
		"Temperature":  temperature,
	})
}

func (h *Handler) PostMessage(c *gin.Context) {
	userEmail := h.userEmail(c)
	chatID := c.Param("id")
	if chatID == "" {
		c.String(http.StatusBadRequest, "missing chat")
		return
	}
	content := strings.TrimSpace(c.PostForm("content"))
	if content == "" {
		var payload struct {
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.String(http.StatusBadRequest, "missing message")
			return
		}
		content = strings.TrimSpace(payload.Content)
	}
	if content == "" {
		c.String(http.StatusBadRequest, "empty message")
		return
	}
	userMessage, err := h.Chat.AppendMessage(c.Request.Context(), userEmail, chatID, "user", content)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to save message")
		return
	}
	model, temperature := h.sessionPreferences(c)
	assistantMessage, usage, err := h.Chat.RunCompletion(c.Request.Context(), userEmail, chatID, model, temperature)
	if err != nil {
		c.String(http.StatusBadRequest, "openai error")
		return
	}
	if acceptsJSON(c.Request.Header) || strings.HasPrefix(c.FullPath(), "/api/") {
		c.JSON(http.StatusOK, gin.H{
			"user":      userMessage,
			"assistant": assistantMessage,
			"usage":     usage,
		})
		return
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("/chat/%s", chatID))
}

func (h *Handler) UpdatePreferences(c *gin.Context) {
	model := strings.TrimSpace(c.PostForm("model"))
	tempValue := strings.TrimSpace(c.PostForm("temperature"))
	if model == "" {
		var payload struct {
			Model       string `json:"model"`
			Temperature string `json:"temperature"`
		}
		if err := c.ShouldBindJSON(&payload); err == nil {
			model = strings.TrimSpace(payload.Model)
			tempValue = strings.TrimSpace(payload.Temperature)
		}
	}
	temperature, err := strconv.ParseFloat(tempValue, 64)
	if err != nil {
		temperature = 0.5
	}
	temperature = clampTemperature(temperature)

	model = h.ensureModel(model)
	session := h.session(c)
	if session == nil {
		c.String(http.StatusInternalServerError, "session unavailable")
		return
	}
	if model != "" {
		session.Values[sessionModel] = model
	}
	session.Values[sessionTemperature] = temperature
	if err := session.Save(c.Request, c.Writer); err != nil {
		c.String(http.StatusInternalServerError, "session save failed")
		return
	}
	if acceptsJSON(c.Request.Header) {
		c.JSON(http.StatusOK, gin.H{"model": model, "temperature": temperature})
		return
	}
	c.Redirect(http.StatusFound, "/")
}

func (h *Handler) session(c *gin.Context) *sessions.Session {
	name := sessionName(h.Config.InstanceName)
	session, err := h.Sessions.Get(c.Request, name)
	if err != nil {
		return nil
	}
	return session
}

func (h *Handler) userEmail(c *gin.Context) string {
	session := h.session(c)
	if session == nil {
		return ""
	}
	if value, ok := session.Values[sessionUserEmail].(string); ok {
		return value
	}
	return ""
}

func (h *Handler) getSessionChatID(c *gin.Context) (string, bool) {
	session := h.session(c)
	if session == nil {
		return "", false
	}
	value, ok := session.Values[sessionChatID].(string)
	return value, ok
}

func (h *Handler) setSessionChatID(c *gin.Context, chatID string) error {
	session := h.session(c)
	if session == nil {
		return fmt.Errorf("session unavailable")
	}
	session.Values[sessionChatID] = chatID
	return session.Save(c.Request, c.Writer)
}

func (h *Handler) sessionPreferences(c *gin.Context) (string, float64) {
	model := ""
	temperature := 0.5
	session := h.session(c)
	if session == nil {
		return model, temperature
	}
	if value, ok := session.Values[sessionModel].(string); ok {
		model = value
	}
	if value, ok := session.Values[sessionTemperature].(float64); ok {
		temperature = value
	} else if value, ok := session.Values[sessionTemperature].(string); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			temperature = parsed
		}
	}
	return h.ensureModel(model), clampTemperature(temperature)
}

func acceptsJSON(header http.Header) bool {
	accept := header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func clampTemperature(value float64) float64 {
	if value < 0.1 {
		return 0.1
	}
	if value > 1.0 {
		return 1.0
	}
	return value
}

func sessionName(instance string) string {
	clean := strings.TrimSpace(strings.ToLower(instance))
	if clean == "" {
		return "session"
	}
	clean = strings.ReplaceAll(clean, " ", "-")
	return fmt.Sprintf("%s-session", clean)
}

func randomState() string {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(nonce)
}

func (h *Handler) ensureModel(model string) string {
	if model == "" {
		if len(h.Config.OpenAI.Models) > 0 {
			return h.Config.OpenAI.Models[0]
		}
		return ""
	}
	for _, allowed := range h.Config.OpenAI.Models {
		if model == allowed {
			return model
		}
	}
	if len(h.Config.OpenAI.Models) > 0 {
		return h.Config.OpenAI.Models[0]
	}
	return model
}
