package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Models  []string
}

type Config struct {
	Port         string
	RedisURL     string
	SessionKey   string
	InstanceName string
	OAuthGoogle  OAuthConfig
	OAuthGitHub  OAuthConfig
	OpenAI       OpenAIConfig
}

func Load() (Config, error) {
	rootDir, err := findRepoRoot()
	if err != nil {
		return Config{}, err
	}
	if err := loadEnvFile(filepath.Join(rootDir, ".env")); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:         getEnv("PORT", "8080"),
		RedisURL:     os.Getenv("REDIS_URL"),
		SessionKey:   os.Getenv("SESSION_KEY"),
		InstanceName: getEnv("INSTANCE_NAME", ""),
		OAuthGoogle: OAuthConfig{
			ClientID:     os.Getenv("OAUTH_GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_GOOGLE_REDIRECT_URL"),
		},
		OAuthGitHub: OAuthConfig{
			ClientID:     os.Getenv("OAUTH_GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_GITHUB_REDIRECT_URL"),
		},
		OpenAI: OpenAIConfig{
			BaseURL: os.Getenv("OPENAI_API_BASE_URL"),
			APIKey:  os.Getenv("OPENAI_API_KEY"),
			Models:  splitCSV(os.Getenv("OPENAI_API_MODELS")),
		},
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var missing []string
	if c.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if c.SessionKey == "" {
		missing = append(missing, "SESSION_KEY")
	}
	if c.InstanceName == "" {
		missing = append(missing, "INSTANCE_NAME")
	}
	if c.OAuthGoogle.ClientID == "" || c.OAuthGoogle.ClientSecret == "" || c.OAuthGoogle.RedirectURL == "" {
		missing = append(missing, "OAUTH_GOOGLE_CLIENT_ID", "OAUTH_GOOGLE_CLIENT_SECRET", "OAUTH_GOOGLE_REDIRECT_URL")
	}
	if c.OAuthGitHub.ClientID == "" || c.OAuthGitHub.ClientSecret == "" || c.OAuthGitHub.RedirectURL == "" {
		missing = append(missing, "OAUTH_GITHUB_CLIENT_ID", "OAUTH_GITHUB_CLIENT_SECRET", "OAUTH_GITHUB_REDIRECT_URL")
	}
	if c.OpenAI.BaseURL == "" {
		missing = append(missing, "OPENAI_API_BASE_URL")
	}
	if c.OpenAI.APIKey == "" {
		missing = append(missing, "OPENAI_API_KEY")
	}
	if len(c.OpenAI.Models) == 0 {
		missing = append(missing, "OPENAI_API_MODELS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func RepoRoot() (string, error) {
	return findRepoRoot()
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var cleaned []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read .env: %w", err)
	}
	return nil
}

func findRepoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := start
	for {
		agentsPath := filepath.Join(current, "AGENTS.md")
		goModPath := filepath.Join(current, "go.mod")
		if fileExists(agentsPath) && fileExists(goModPath) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("unable to locate repo root from %s", start)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
