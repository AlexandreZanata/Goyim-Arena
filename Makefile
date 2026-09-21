# Goyim Arena — comandos canônicos de validação (P01-T03).
#
# Regra do plano: um gate ainda não implementado falha com mensagem
# explícita e nunca retorna sucesso falso. `make verify` passa apenas
# para as capacidades já existentes e lista claramente as pendentes.

GO ?= go
GOFMT ?= gofmt
NPM ?= npm
K6 ?= k6
SQLC ?= $(shell which sqlc 2>/dev/null || echo "$(shell $(GO) env GOPATH)/bin/sqlc")
ASSETGEN := $(GO) run ./cmd/assetgen

# Gerador de contratos TypeScript (P18-T02): lê o subconjunto versionado do
# OpenAPI e emite web/src/contracts/generated.ts (nunca editado à mão).
CONTRACTGEN := $(GO) run ./tools/contractgen

# Imagem de produção (P19-T01): receita em Dockerfile, auditoria do artefato em
# tools/imageaudit. IMAGE é a tag que o build usa e que o scan examina.
IMAGE ?= goyim-arena:local
TRIVY ?= trivy

.PHONY: fmt fmt-check test-unit test-integration test-security test-web typecheck build-web audit-web audit-i18n test-contract test-e2e test-load-smoke image-build image-verify image-scan generate generate-check verify

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
#
# A segunda invocação é o build com a tag pseudolocale (P18-T10): é o catálogo
# derivado que as jornadas de navegador dirigem, e ele tem testes próprios
# (registro no allowlist, formatação de placeholders, catálogos reais
# intactos). Sem ela, a tag só seria compilada dentro do harness — e um erro no
# registro apareceria como jornada vermelha em vez de teste vermelho.
test-unit:
	$(GO) test ./...
	$(GO) test -tags pseudolocale ./internal/i18n/...
	@echo "test-unit: ok"

# test-integration executa os testes de integração contra PostgreSQL real descartável (P03-T05, P03-T06).
test-integration:
	$(GO) test -v -race ./internal/platform/dbpool/... ./internal/platform/dbtest/... ./internal/platform/postgres/...
	@echo "test-integration: ok"

# test-web compila o frontend e seus testes com o tsc oficial (strict) e os
# executa no runner nativo do Node. Nenhuma dependência nova: o runtime
# entregue ao browser continua sem terceiros, e o harness vive fora de web/src.
test-web:
	$(NPM) ci --prefix web
	$(NPM) --prefix web run test
	@echo "test-web: ok"

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
	@rm -rf web/dist
	$(ASSETGEN) -input web/generated -input web/src -output web/dist -manifest web/dist/manifest.json
	@echo "build-web: ok"

# audit-web mede o build entregue contra os orçamentos e as regras de
# dependência do frontend (P18-T08, docs/FRONTEND.md §11): custo comprimido por
# página pública e do CSS inicial, imports externos, bare specifiers, imports
# não publicados, construtos que a CSP servida recusa e primitivas de rede fora
# de web/src/core. Ele depende de build-web porque mede o build que as páginas
# referenciam, nunca as fontes: a árvore que compila e o build que é servido são
# dois artefatos diferentes.
audit-web: build-web
	$(GO) run ./tools/webaudit -build web/dist
	@echo "audit-web: ok"

# audit-i18n varre a árvore entregue em busca do que o catálogo não pode
# garantir sozinho (P18-T10): documento que escreve a própria linguagem (ou
# nenhuma), documento que traz prosa fora do catálogo e folha de estilo com
# propriedade física. Não depende de build porque lê o código-fonte entregue:
# é a árvore que vai para produção, e um layout é uma propriedade dela, não do
# artefato compilado. O scanner está fora do pacote entregue e nunca escreve
# nada; a lista de regras e o porquê de cada uma estão em tools/i18naudit.
audit-i18n:
	$(GO) run ./tools/i18naudit -root .
	@echo "audit-i18n: ok"

# image-build constrói a imagem de produção a partir do Dockerfile. As bases
# estão fixadas por digest, então o mesmo commit gera a mesma árvore.
image-build:
	docker build --file Dockerfile --tag $(IMAGE) .
	@echo "image-build: ok"

# image-verify é o gate da imagem (P19-T01): constrói, sobe o container com
# filesystem somente leitura contra um PostgreSQL descartável, prova que ele
# aplica as próprias migrations, serve uma página e o asset com hash que ela
# referencia, e entrega a receita e o artefato a tools/imageaudit. Ele não entra
# em `verify` porque exige um daemon Docker — o mesmo motivo de test-e2e.
image-verify:
	ARENA_IMAGE=$(IMAGE) tools/imageaudit/verify.sh
	@echo "image-verify: ok"

# image-scan procura vulnerabilidades conhecidas na imagem construída. Ele exige
# um scanner instalado fora do repositório (o padrão é trivy), exatamente como
# test-load-smoke exige k6; sem ele o alvo falha explicitamente e nunca retorna
# sucesso falso. A base distroless não traz gerenciador de pacotes, então o que
# o scanner examina é sobretudo o binário Go e seus módulos.
image-scan: image-build
	@command -v "$(TRIVY)" >/dev/null 2>&1 || (echo "image-scan: trivy is required; install it outside the repository" >&2; exit 1)
	$(TRIVY) image --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed --exit-code 1 $(IMAGE)
	@echo "image-scan: ok"

# test-contract valida o contrato OpenAPI versionado: o documento parseia,
# satisfaz as convenções estruturais do plano (Problem Details, security
# schemes, paginação/idempotência) e casa com as rotas registradas pelo
# binário — drift de rota falha o build (P02-T06).
test-contract:
	$(GO) test ./internal/contract/...
	@echo "test-contract: ok"

# generate valida os catálogos i18n, reescreve os artefatos gerados, emite os
# contratos TypeScript do OpenAPI e executa a geração de código SQL tipado com
# sqlc para o adapter PostgreSQL.
generate:
	$(I18NGEN)
	$(CONTRACTGEN)
	$(SQLC) generate
	@echo "generate: ok"

# generate-check valida os catálogos i18n, a ausência de drift nos contratos
# TypeScript e no código SQL gerado pelo sqlc, falhando caso os artefatos
# gerados estejam desatualizados.
generate-check:
	$(I18NGEN) -check
	$(CONTRACTGEN) -check
	$(SQLC) diff
	@echo "generate-check: ok"

# test-security executa as regressões críticas do threat model: cache leak,
# IDOR/ownership, CSRF, replay de webhook, double spend e bypass administrativo.
# A matriz estrutural em internal/security também exige evidência para cada THR-*.
test-security:
	$(GO) test -count=1 ./internal/security/... ./internal/platform/security/... ./internal/arguments/adapters/http/... ./internal/arguments/adapters/postgres/... ./internal/billing/adapters/stripe/... ./internal/billing/application/... ./internal/moderation/adapters/http/... ./internal/moderation/application/... ./internal/positions/adapters/http/... ./internal/transparency/adapters/http/... ./internal/wallet/adapters/http/... ./internal/wallet/adapters/postgres/...
	@echo "test-security: ok"

# test-e2e roda as jornadas críticas em navegador real (P18-T07). O harness
# descartável de tools/e2e provisiona um PostgreSQL próprio, um sink de email em
# diretório, o binário `arena server` e o runner pinado — tudo fora do pacote
# entregue. Antes das jornadas, o gate de isolamento prova que nada do runner
# está em web/, no build que as páginas referenciam ou no binário entregue.
#
# O alvo depende de build-web porque são as páginas reais, servindo o build
# real, que as jornadas dirigem.
#
# Ele não está em `verify` porque exige um navegador instalado na máquina
# (o harness garante o build do Chromium que o runner fixa, mas não instala
# dependências de sistema). Ausente do alvo, nunca ausente de gate.
#
# Exige ARENA_DATABASE_URL (o workflow `verify` já a define em todo o job). O
# banco nomeado por ela é apenas a porta: o harness cria um banco descartável
# ao lado dele, migra, semeia e **remove** — o banco apontado nunca é tocado.
test-e2e: build-web
	@tools/e2e/isolation-check.sh
	tools/e2e/harness.sh
	@echo "test-e2e: ok"

# test-load-smoke executa os cenários k6 versionados contra uma instância
# local preparada exclusivamente com dados sintéticos. O gate falha se k6 não
# estiver instalado ou se o workload não atingir os thresholds declarados.
test-load-smoke:
	@test -n "$(K6_BASE_URL)" || (echo "test-load-smoke: K6_BASE_URL is required" >&2; exit 1)
	@command -v "$(K6)" >/dev/null 2>&1 || (echo "test-load-smoke: k6 is required; install it outside the repository" >&2; exit 1)
	@report="$$(mktemp)"; trap 'rm -f "$$report"' EXIT; \
	printf 'load-smoke report: commit=%s host=%s kernel=%s cpu=%s dataset=%s config=%s\n' \
		"$$(git rev-parse --short HEAD)" "$$(hostname)" "$$(uname -sr)" "$$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)" \
		"$${K6_DATASET_SEED:-unset}" "$${K6_BASE_URL}"; \
	"$(K6)" run --summary-export "$$report" tests/load/smoke.js; \
	printf 'load-smoke report: summary=%s\n' "$$report"

# verify agrega os gates existentes do estágio atual e lista os pendentes.
# Gates pendentes nunca são executados aqui: eles falham explicitamente
# quando invocados diretamente e nunca retornam sucesso falso.
verify: fmt-check generate-check test-unit test-integration test-contract test-security test-web typecheck build-web audit-web audit-i18n
	@echo "verify: gates ainda não criados (invocar falha explicitamente, nunca retorna sucesso falso):"
	@for gate in lint test-race vuln; do \
		echo "  - $$gate"; \
	done
	@echo "verify: gates criados que exigem ambiente próprio e por isso não entram neste alvo:"
	@for gate in test-e2e test-load-smoke image-verify image-scan; do \
		echo "  - $$gate"; \
	done
	@echo "verify: OK — todas as capacidades existentes do estágio atual passaram."
