start-db:
	docker start 4e014fe7c4ec
	@echo "DB container started"

run-dev:
	@echo "Starting development server"
	@echo "Checking if DB container is running..."
	@docker ps --filter "id=4e014fe7c4ec" --format "{{.ID}}" | grep -q 4e014fe7c4ec || (echo "DB container not running, starting..." && docker start 4e014fe7c4ec)
	@echo "DB container is running"
	air .
