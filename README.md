# SmartChat
A simple chat interface to AI models via OpenAI API. It works well with LocalAI.

## Local run

1. Create a `.env` in the repo root (environment variables always override `.env`):

```
PORT=8080
INSTANCE_NAME=SmartChat
REDIS_URL=redis://localhost:6379/0
SESSION_KEY=replace-with-32+chars

OAUTH_GOOGLE_CLIENT_ID=...
OAUTH_GOOGLE_CLIENT_SECRET=...
OAUTH_GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback

OAUTH_GITHUB_CLIENT_ID=...
OAUTH_GITHUB_CLIENT_SECRET=...
OAUTH_GITHUB_REDIRECT_URL=http://localhost:8080/auth/github/callback

OPENAI_API_BASE_URL=https://local-ai.local:32217/v1
OPENAI_API_KEY=...
OPENAI_API_MODELS=llama-3.2-1b-instruct:q8_0,another-model
ALLOWED_USERS=person1@example.com|person2@example.com
```

2. Run the server:

```
go run ./cmd/server
```

3. Open `http://localhost:8080` in your browser.

Notes:
- OAuth callbacks must match the URLs configured in Google/GitHub consoles.
- Session cookies are Secure/HttpOnly/SameSite=Lax, so use HTTPS if your browser blocks Secure cookies on `http://`.
