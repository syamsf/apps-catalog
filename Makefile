DATABASE_URL ?= postgres://portal:portal@localhost:5432/portal?sslmode=disable
export DATABASE_URL
# store tests empty every table, so they use their own database
TEST_DATABASE_URL ?= postgres://portal:portal@localhost:5432/portal_test?sslmode=disable

AIR = go tool -modfile=tools/go.mod air

.PHONY: db run dev debug import web web-dev docs test build docker tabler-icons

db:
	docker compose up -d --wait db
	docker compose exec db sh -c "psql -U portal -tc \"SELECT 1 FROM pg_database WHERE datname = 'portal_test'\" | grep -q 1 || createdb -U portal portal_test"

run:
	LOG_FORMAT=console go run ./cmd/portal serve

# hot reload with debug logs
dev:
	LOG_LEVEL=debug LOG_FORMAT=console $(AIR) -c .air.toml

# hot reload under delve, attach a debugger to localhost:2345
debug:
	LOG_LEVEL=debug LOG_FORMAT=console $(AIR) -c .air.debug.toml

# add the apps in ./catalog once, existing apps are skipped
import:
	go run ./cmd/portal import ./catalog

web:
	cd web && npm ci && npm run build

web-dev:
	cd web && npm run dev

docs:
	go generate ./internal/api

# refresh internal/icons/tabler.json.gz, pick a release at least two weeks old
TABLER_VERSION ?= 3.49.0
tabler-icons:
	rm -rf .tabler && mkdir .tabler
	cd .tabler && npm pack --silent @tabler/icons@$(TABLER_VERSION) >/dev/null && tar xzf *.tgz
	cd internal/icons && go run generate.go ../../.tabler/package && cp ../../.tabler/package/LICENSE TABLER_LICENSE
	rm -rf .tabler

test:
	go vet ./...
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test ./...
	cd web && npm run type-check

build: web
	CGO_ENABLED=0 go build -o bin/portal ./cmd/portal

docker:
	docker build -t apps-catalog:dev .
