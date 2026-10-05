LINT_IMAGE := golangci/golangci-lint:v2.14.0
# The SQL tests run when this is set; make postgres-up starts the database it points at.
export RECON_TEST_PG_DSN ?= postgres://recon:recon@127.0.0.1:55433/recon

.PHONY: test vet fmt lint build check postgres-up postgres-down

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	docker run --rm -v $(CURDIR):/src -w /src $(LINT_IMAGE) golangci-lint run

build:
	go build -o bin/recon ./cmd/recon

check: vet test lint

postgres-up:
	docker run -d --rm --name recon-pg -e POSTGRES_USER=recon -e POSTGRES_PASSWORD=recon -e POSTGRES_DB=recon -p 55433:5432 postgres:17-alpine
	until docker exec recon-pg pg_isready -U recon >/dev/null 2>&1; do sleep 1; done

postgres-down:
	docker rm -f recon-pg
