FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o api ./cmd/api

# ---- Final image ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

RUN mkdir -p logs media && chown -R appuser:appgroup /app

COPY --from=builder /app/api .

USER appuser

EXPOSE 8080

CMD ["./api"]
