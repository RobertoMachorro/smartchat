package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"robertomachorro/smartchat/internal/config"
)

type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderGitHub Provider = "github"
)

type Service struct {
	GoogleConfig *oauth2.Config
	GitHubConfig *oauth2.Config
}

func NewService(cfg config.Config) *Service {
	googleConfig := &oauth2.Config{
		ClientID:     cfg.OAuthGoogle.ClientID,
		ClientSecret: cfg.OAuthGoogle.ClientSecret,
		RedirectURL:  cfg.OAuthGoogle.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	githubConfig := &oauth2.Config{
		ClientID:     cfg.OAuthGitHub.ClientID,
		ClientSecret: cfg.OAuthGitHub.ClientSecret,
		RedirectURL:  cfg.OAuthGitHub.RedirectURL,
		Scopes:       []string{"user:email"},
		Endpoint:     github.Endpoint,
	}
	return &Service{GoogleConfig: googleConfig, GitHubConfig: githubConfig}
}

func (s *Service) AuthURL(provider Provider, state string) (string, error) {
	switch provider {
	case ProviderGoogle:
		return s.GoogleConfig.AuthCodeURL(state, oauth2.AccessTypeOnline), nil
	case ProviderGitHub:
		return s.GitHubConfig.AuthCodeURL(state), nil
	default:
		return "", fmt.Errorf("unsupported provider")
	}
}

func (s *Service) Exchange(ctx context.Context, provider Provider, code string) (*oauth2.Token, error) {
	switch provider {
	case ProviderGoogle:
		return s.GoogleConfig.Exchange(ctx, code)
	case ProviderGitHub:
		return s.GitHubConfig.Exchange(ctx, code)
	default:
		return nil, fmt.Errorf("unsupported provider")
	}
}

func (s *Service) FetchEmail(ctx context.Context, provider Provider, token *oauth2.Token) (string, error) {
	switch provider {
	case ProviderGoogle:
		return fetchGoogleEmail(ctx, s.GoogleConfig, token)
	case ProviderGitHub:
		return fetchGitHubEmail(ctx, s.GitHubConfig, token)
	default:
		return "", fmt.Errorf("unsupported provider")
	}
}

func fetchGoogleEmail(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (string, error) {
	client := cfg.Client(ctx, token)
	response, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", fmt.Errorf("google userinfo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("google userinfo status %d", response.StatusCode)
	}
	var data struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("decode google userinfo: %w", err)
	}
	if data.Email == "" {
		return "", fmt.Errorf("google email missing")
	}
	return data.Email, nil
}

func fetchGitHubEmail(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (string, error) {
	client := cfg.Client(ctx, token)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("github request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("github user emails: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("github user emails status %d", response.StatusCode)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode github user emails: %w", err)
	}
	for _, entry := range emails {
		if entry.Primary && entry.Verified && strings.Contains(entry.Email, "@") {
			return entry.Email, nil
		}
	}
	for _, entry := range emails {
		if strings.Contains(entry.Email, "@") {
			return entry.Email, nil
		}
	}
	return "", fmt.Errorf("github email missing")
}
