.PHONY: dev build clean run-dummy

dev:
	docker compose up --build

build:
	docker compose build

clean:
	docker compose down -v

run-dummy:
	docker compose up -d dummy-postgres
