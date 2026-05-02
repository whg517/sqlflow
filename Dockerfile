# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o /sqlflow ./cmd/server/

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /sqlflow /app/sqlflow
COPY config/config.yaml /app/config.yaml

RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["/app/sqlflow"]
