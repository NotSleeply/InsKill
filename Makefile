.PHONY: build run test vet up integration

build:
	go build -o bin/inskill ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

up:
	docker compose -f deploy/docker-compose.yml up -d --build

integration:
	go test -tags integration ./integration/ -v
