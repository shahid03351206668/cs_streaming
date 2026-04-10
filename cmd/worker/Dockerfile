FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o worker ./cmd/worker

# ---- Final image ----
FROM alpine:3.19

# ffmpeg is required for HLS transcoding and thumbnail extraction
RUN apk add --no-cache ca-certificates tzdata ffmpeg

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

RUN mkdir -p logs media && chown -R appuser:appgroup /app

COPY --from=builder /app/worker .

USER appuser

CMD ["./worker"]
