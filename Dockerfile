FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files first (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# ---- Final stage ----
FROM alpine:latest

WORKDIR /app

# Copy binary and templates
COPY --from=builder /app/server .
COPY templates/ templates/

# Create data directory (will be overridden by Railway volume)
RUN mkdir -p data

EXPOSE 8080

CMD ["./server"]
