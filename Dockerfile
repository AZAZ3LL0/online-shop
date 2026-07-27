FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o qzq-kiim ./cmd/server

FROM alpine:3.19
WORKDIR /app
# Run as an unprivileged user, not root.
RUN adduser -D -u 10001 appuser
COPY --from=builder /app/qzq-kiim .
COPY templates/ ./templates/
COPY static/ ./static/
COPY migrations/ ./migrations/
USER appuser
EXPOSE 8080
CMD ["./qzq-kiim"]
