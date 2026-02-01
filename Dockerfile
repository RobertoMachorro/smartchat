FROM golang:1.25.6-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
COPY AGENTS.md ./AGENTS.md
COPY README.md ./README.md

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/server /app/server
COPY --from=build /app/web /app/web
COPY --from=build /app/AGENTS.md /app/AGENTS.md
COPY --from=build /app/go.mod /app/go.mod

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/server"]
