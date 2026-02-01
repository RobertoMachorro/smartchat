package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"

	"robertomachorro/smartchat/internal/config"
	"robertomachorro/smartchat/internal/handler"
	"robertomachorro/smartchat/internal/service/auth"
	"robertomachorro/smartchat/internal/service/chat"
	"robertomachorro/smartchat/internal/service/openai"
	"robertomachorro/smartchat/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	rootDir, err := config.RepoRoot()
	if err != nil {
		log.Fatalf("root error: %v", err)
	}

	redisStore, err := store.NewRedisStore(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis error: %v", err)
	}

	aiClient := openai.NewClient(cfg.OpenAI.BaseURL, cfg.OpenAI.APIKey)
	chatService := chat.NewService(redisStore.Client, aiClient)
	authService := auth.NewService(cfg)

	sessionStore := sessions.NewCookieStore([]byte(cfg.SessionKey))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.SetHTMLTemplate(loadTemplates(rootDir))
	router.Static("/static", filepath.Join(rootDir, "web", "static"))

	h := handler.NewHandler(cfg, sessionStore, authService, chatService)
	h.RegisterRoutes(router)

	if err := router.Run("0.0.0.0:" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadTemplates(rootDir string) *template.Template {
	tmpl := template.New("").Funcs(template.FuncMap{
		"formatUTC": func(value time.Time) string {
			return value.UTC().Format(time.RFC3339)
		},
	})
	template.Must(tmpl.ParseGlob(filepath.Join(rootDir, "web", "templates", "*.html")))
	template.Must(tmpl.ParseGlob(filepath.Join(rootDir, "web", "templates", "partials", "*.html")))
	return tmpl
}
