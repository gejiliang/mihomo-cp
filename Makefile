.PHONY: build build-frontend build-backend dev test clean docker

build: build-frontend build-backend

build-frontend:
	cd web && pnpm install && pnpm build

build-backend:
	go build -o mihomo-cp ./cmd/mihomo-cp

dev:
	@echo "Start backend: go run ./cmd/mihomo-cp"
	@echo "Start frontend: cd web && pnpm dev"

test:
	go test ./... -v

clean:
	rm -f mihomo-cp
	rm -rf web/dist

docker:
	docker build -t mihomo-cp .
