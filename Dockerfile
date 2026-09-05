# ── Stage 1: Build UI ────────────────────────────────────────────────────────
FROM node:22-alpine AS ui-builder

WORKDIR /ui
COPY ui/package*.json ./
RUN npm ci 
COPY ui/ ./
RUN npm run build

# ── Stage 2: Build Go binary ─────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /ui/dist ./ui/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags embed -trimpath -ldflags="-s -w" -o /alatyr .

# ── Stage 3: Final image ──────────────────────────────────────────────────────
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app

COPY --from=go-builder --chown=app:app /alatyr /usr/local/bin/alatyr

USER app

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/alatyr"]
