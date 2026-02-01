package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"robertomachorro/smartchat/internal/service/openai"
)

type Service struct {
	Redis *redis.Client
	AI    *openai.Client
}

type ChatSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type ChatView struct {
	Summary  ChatSummary
	Messages []Message
}

func NewService(redisClient *redis.Client, aiClient *openai.Client) *Service {
	return &Service{Redis: redisClient, AI: aiClient}
}

func (s *Service) EnsureChat(ctx context.Context, userEmail string) (ChatSummary, error) {
	summaries, err := s.ListChats(ctx, userEmail)
	if err != nil {
		return ChatSummary{}, err
	}
	if len(summaries) > 0 {
		return summaries[0], nil
	}
	return s.NewChat(ctx, userEmail, "New chat")
}

func (s *Service) NewChat(ctx context.Context, userEmail, title string) (ChatSummary, error) {
	chatID := uuid.NewString()
	if strings.TrimSpace(title) == "" {
		title = "New chat"
	}
	summary := ChatSummary{
		ID:        chatID,
		Title:     title,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.saveChatMeta(ctx, userEmail, summary); err != nil {
		return ChatSummary{}, err
	}
	return summary, nil
}

func (s *Service) ListChats(ctx context.Context, userEmail string) ([]ChatSummary, error) {
	ids, err := s.Redis.LRange(ctx, userChatsKey(userEmail), 0, 19).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	summaries := make([]ChatSummary, 0, len(ids))
	for _, id := range ids {
		data, err := s.Redis.Get(ctx, chatMetaKey(id)).Result()
		if err != nil {
			continue
		}
		var summary ChatSummary
		if err := json.Unmarshal([]byte(data), &summary); err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (s *Service) GetChat(ctx context.Context, userEmail, chatID string) (ChatView, error) {
	if ok, err := s.verifyOwner(ctx, userEmail, chatID); err != nil {
		return ChatView{}, err
	} else if !ok {
		return ChatView{}, fmt.Errorf("not authorized")
	}

	metaData, err := s.Redis.Get(ctx, chatMetaKey(chatID)).Result()
	if err != nil {
		return ChatView{}, err
	}
	var summary ChatSummary
	if err := json.Unmarshal([]byte(metaData), &summary); err != nil {
		return ChatView{}, err
	}
	messages, err := s.fetchMessages(ctx, chatID)
	if err != nil {
		return ChatView{}, err
	}
	return ChatView{Summary: summary, Messages: messages}, nil
}

func (s *Service) AppendMessage(ctx context.Context, userEmail, chatID, role, content string) (Message, error) {
	if ok, err := s.verifyOwner(ctx, userEmail, chatID); err != nil {
		return Message{}, err
	} else if !ok {
		return Message{}, fmt.Errorf("not authorized")
	}
	message := Message{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return Message{}, err
	}
	if err := s.Redis.RPush(ctx, chatMessagesKey(chatID), payload).Err(); err != nil {
		return Message{}, err
	}
	if err := s.touchChat(ctx, userEmail, chatID, content); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) RunCompletion(ctx context.Context, userEmail, chatID, model string, temperature float64) (Message, openai.Usage, error) {
	if ok, err := s.verifyOwner(ctx, userEmail, chatID); err != nil {
		return Message{}, openai.Usage{}, err
	} else if !ok {
		return Message{}, openai.Usage{}, fmt.Errorf("not authorized")
	}
	messages, err := s.fetchMessages(ctx, chatID)
	if err != nil {
		return Message{}, openai.Usage{}, err
	}
	aiMessages := make([]openai.Message, 0, len(messages))
	for _, message := range messages {
		aiMessages = append(aiMessages, openai.Message{Role: message.Role, Content: message.Content})
	}
	response, usage, err := s.AI.ChatCompletion(ctx, model, aiMessages, temperature)
	if err != nil {
		return Message{}, openai.Usage{}, err
	}
	stored := Message{
		Role:      response.Role,
		Content:   response.Content,
		CreatedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return Message{}, openai.Usage{}, err
	}
	if err := s.Redis.RPush(ctx, chatMessagesKey(chatID), payload).Err(); err != nil {
		return Message{}, openai.Usage{}, err
	}
	if err := s.touchChat(ctx, userEmail, chatID, response.Content); err != nil {
		return Message{}, openai.Usage{}, err
	}
	return stored, usage, nil
}

func (s *Service) fetchMessages(ctx context.Context, chatID string) ([]Message, error) {
	values, err := s.Redis.LRange(ctx, chatMessagesKey(chatID), 0, -1).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	messages := make([]Message, 0, len(values))
	for _, value := range values {
		var message Message
		if err := json.Unmarshal([]byte(value), &message); err != nil {
			continue
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *Service) touchChat(ctx context.Context, userEmail, chatID, lastContent string) error {
	metaData, err := s.Redis.Get(ctx, chatMetaKey(chatID)).Result()
	if err != nil {
		return err
	}
	var summary ChatSummary
	if err := json.Unmarshal([]byte(metaData), &summary); err != nil {
		return err
	}
	if summary.Title == "New chat" && strings.TrimSpace(lastContent) != "" {
		summary.Title = summarizeTitle(lastContent)
	}
	summary.UpdatedAt = time.Now().UTC()
	return s.saveChatMeta(ctx, userEmail, summary)
}

func (s *Service) saveChatMeta(ctx context.Context, userEmail string, summary ChatSummary) error {
	payload, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	pipe := s.Redis.TxPipeline()
	pipe.Set(ctx, chatMetaKey(summary.ID), payload, 0)
	pipe.Set(ctx, chatOwnerKey(summary.ID), userEmail, 0)
	pipe.LRem(ctx, userChatsKey(userEmail), 0, summary.ID)
	pipe.LPush(ctx, userChatsKey(userEmail), summary.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Service) verifyOwner(ctx context.Context, userEmail, chatID string) (bool, error) {
	owner, err := s.Redis.Get(ctx, chatOwnerKey(chatID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner == userEmail, nil
}

func summarizeTitle(content string) string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 32 {
		trimmed = trimmed[:32]
	}
	return trimmed
}

func userChatsKey(email string) string {
	return fmt.Sprintf("userchats:%s", email)
}

func chatMetaKey(chatID string) string {
	return fmt.Sprintf("chatmeta:%s", chatID)
}

func chatMessagesKey(chatID string) string {
	return fmt.Sprintf("chatmessages:%s", chatID)
}

func chatOwnerKey(chatID string) string {
	return fmt.Sprintf("chatowner:%s", chatID)
}
