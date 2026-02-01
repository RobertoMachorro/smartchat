FROM golang:1.25.5-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /app/server /app/server
COPY --from=build /app/web /app/web

EXPOSE 8080

ENTRYPOINT ["/app/server"]
