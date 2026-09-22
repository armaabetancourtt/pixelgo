.PHONY: infra-up infra-down server-test server-run android-test contract

infra-up:
	docker compose -f infrastructure/docker-compose.yml up -d

infra-down:
	docker compose -f infrastructure/docker-compose.yml down

server-test:
	cd server && go test ./...

server-run:
	cd server && go run ./cmd/api

android-test:
	cd android && gradle :app:testDebugUnitTest

contract:
	docker run --rm -v "$$(pwd):/spec" redocly/cli lint /spec/contracts/openapi.yaml
