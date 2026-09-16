# Goyim Arena — comandos canônicos de validação (P01-T03).
#
# Regra do plano: um gate ainda não implementado falha com mensagem
# explícita e nunca retorna sucesso falso. `make verify` passa apenas
# para as capacidades já existentes e lista claramente as pendentes.

GO ?= go
GOFMT ?= gofmt
NPM ?= npm

.PHONY: fmt fmt-check test-unit typecheck build-web generate generate-check verify

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

# generate é um gate ainda não implementado: o gerador de contratos
# (api/openapi.json -> web/src/contracts/generated.ts) chega em fase posterior.
generate:
	@echo "generate: FALHOU — gate ainda não implementado (geração de contratos OpenAPI ainda não existe no estágio atual)." >&2
	@exit 1

# generate-check é um gate ainda não implementado: sem gerador, não há
# artefato para comparar.
generate-check:
	@echo "generate-check: FALHOU — gate ainda não implementado (nenhuma capacidade de geração para validar)." >&2
	@exit 1

# verify agrega os gates existentes do estágio atual e lista os pendentes.
# Gates pendentes nunca são executados aqui: eles falham explicitamente
# quando invocados diretamente e nunca retornam sucesso falso.
verify: fmt-check test-unit typecheck build-web
	@echo "verify: gates presentes, porém não implementados (falham explicitamente ao serem invocados):"
	@echo "  - generate"
	@echo "  - generate-check"
	@echo "verify: gates ainda não criados:"
	@for gate in lint test-integration test-contract test-security test-e2e test-race test-load-smoke vuln; do \
		echo "  - $$gate"; \
	done
	@echo "verify: OK — todas as capacidades existentes do estágio atual passaram."
