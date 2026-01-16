start-db:
	docker start docker-pg
	@echo "DB container started"

redis:
	docker start tasksy-redis || docker run --name tasksy-redis -p 6379:6379 -d redis:alpine

worker:
	go run cmd/worker/main.go

server:
	go run cmd/api/main.go

run-all: redis
	@echo "Starting Worker and Server..."
	@trap 'kill %1; kill %2' SIGINT; \
	go run cmd/worker/main.go & \
	go run main.go