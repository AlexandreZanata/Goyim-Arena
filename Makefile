# Goyim Arena — comandos canônicos de validação (P01-T03).
#
# Regra do plano: um gate ainda não implementado falha com mensagem
# explícita e nunca retorna sucesso falso. `make verify` passa apenas
# para as capacidades já existentes e lista claramente as pendentes.

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
SQLC ?= $(shell which sqlc 2>/dev/null || echo "$(shell $(GO) env GOPATH)/bin/sqlc")

.PHONY: fmt fmt-check test-unit test-integration typecheck build-web test-contract generate generate-check verify

# Gerador i18n (P02-T07): fontes em locales/, artefatos versionados em
# web/src/i18n/generated.ts e internal/i18n/generated.go (nunca editados).
I18NGEN := $(GO) run ./cmd/i18ngen

# fmt formata o código Go (alvo mutante; use fmt-check para validar sem alterar).
fmt:
	$(GOFMT) -w .

# fmt-check valida a formatação do Go sem alterar nenhum arquivo.
fmt-check:
	@files="$$($(GOFMT) -l .)"; \
	if [ -n "$$files" ]; then \
		echo "fmt-check: FALHOU — arquivos fora do formato (execute 'make fmt'):" >&2; \
		echo "$$files" >&2; \
		exit 1; \
	fi; \
	echo "fmt-check: ok"

# test-unit executa os testes unitários das capacidades existentes (Go).
# O frontend ainda não possui runner de testes; será agregado quando existir.
test-unit:
	$(GO) test ./...
	@echo "test-unit: ok"

# test-integration executa os testes de integração contra PostgreSQL real descartável (P03-T05, P03-T06).
test-integration:
	$(GO) test -v -race ./internal/platform/dbpool/... ./internal/platform/dbtest/... ./internal/platform/postgres/...
	@echo "test-integration: ok"

# typecheck roda a checagem estrita de tipos do frontend (tsc --noEmit).
# npm ci garante instalação reprodutível a partir do package-lock.json.
typecheck:
	$(NPM) ci --prefix web
	$(NPM) --prefix web run typecheck
	@echo "typecheck: ok"

# build-web compila o frontend para ESM nativo em web/generated/ (ignorado).
build-web:
	$(NPM) ci --prefix web
	$(NPM) --prefix web run build
	@echo "build-web: ok"

# test-contract valida o contrato OpenAPI versionado: o documento parseia,
# satisfaz as convenções estruturais do plano (Problem Details, security
# schemes, paginação/idempotência) e casa com as rotas registradas pelo
# binário — drift de rota falha o build (P02-T06).
test-contract:
	$(GO) test ./internal/contract/...
	@echo "test-contract: ok"

# generate valida os catálogos i18n, reescreve os artefatos gerados
# e executa a geração de código SQL tipado com sqlc para o adapter PostgreSQL.
generate:
	$(I18NGEN)
	$(SQLC) generate
	@echo "generate: ok"

# generate-check valida os catálogos i18n e a ausência de drift no código SQL
# gerado pelo sqlc, falhando caso os artefatos gerados estejam desatualizados.
generate-check:
	$(I18NGEN) -check
	$(SQLC) diff
	@echo "generate-check: ok"

# verify agrega os gates existentes do estágio atual e lista os pendentes.
# Gates pendentes nunca são executados aqui: eles falham explicitamente
# quando invocados diretamente e nunca retornam sucesso falso.
verify: fmt-check generate-check test-unit test-integration test-contract typecheck build-web
	@echo "verify: gates presentes, porém não implementados (falham explicitamente ao serem invocados):"
	@echo "verify: gates ainda não criados:"
	@for gate in lint test-security test-e2e test-race test-load-smoke vuln; do \
		echo "  - $$gate"; \
	done
	@echo "verify: OK — todas as capacidades existentes do estágio atual passaram."
