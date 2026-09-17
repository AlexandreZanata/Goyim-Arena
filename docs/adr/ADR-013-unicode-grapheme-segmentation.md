# ADR-013 — Segmentação de grapheme clusters com `rivo/uniseg`

**Status:** aceito

**Data:** 2026-09-17

## Contexto

O produto mede texto em grapheme clusters, não em runas: o limite de 3.000 clusters por argumento (REQ-ARG-04, BUSINESS_RULES §4) e o custo de 1 INK por cluster publicado (REQ-WAL-02, BUSINESS_RULES §8) precisam contar o que a pessoa percebe como caractere. Emoji ZWJ (`👩🏽‍🚀`, `👨‍👩‍👧‍👦`, `🏳️‍🌈`), acentos combinantes (`e` + U+0301), modificadores de tom de pele e pares de bandeiras (`🇧🇷`) formam um único cluster; `utf8.RuneCountInString` superestima esses casos e divergiria do preço e do limite. A biblioteca padrão do Go (1.27) não expõe segmentação de grapheme clusters, e a P10-T01 proíbe implementar um contador manual do UAX #29.

## Decisão

Admitir `github.com/rivo/uniseg` na versão fixa **v0.4.7** como a biblioteca de segmentação de grapheme clusters do backend, conforme o processo do ADR-011 e a linha provisória já prevista em `DEPENDENCIES.md`:

- **Licença e fornecedor:** MIT (`LICENSE.txt` do módulo), mantido, releases regulares, `go.mod` do módulo declara apenas `go 1.18` — **sem dependências transitivas**.
- **API usada:** `uniseg.GraphemeClusterCount(string) int`, função pura e determinística, sem I/O e sem estado global.
- **Owner único:** `internal/platform/text` (ADR-011 §10.4) expõe `GraphemeCount` e isola o fornecedor; nenhum tipo `uniseg` cruza a fronteira do package.
- **Domínio e aplicação continuam Go puro (AGENTS.md, DEPENDENCIES.md §3.1):** a aplicação define uma porta pequena de contagem e o bootstrap compõe a implementação de `internal/platform/text`. Para tornar a decisão executável, `internal/architecture_test.go` passa a proibir `github.com/rivo/` nas camadas `domain` e `application`.

## Alternativas

- **`utf8.RuneCountInString` (biblioteca padrão):** conta runas, não clusters; erra em emoji ZWJ, combinantes, bandeiras e tom de pele, divergindo do limite e da tarifação; rejeitada.
- **Contador manual do UAX #29:** proibido pela P10-T01 e notoriamente sensível a versões do Unicode; rejeitada.
- **`golang.org/x/text`:** não oferece quebra de grapheme clusters (apenas palavras, linhas e frases); rejeitada.
- **Biblioteca de largura de terminal:** escopo maior do que o necessário e sem ganho para contagem; rejeitada.

## Consequências

- contagem de texto alinhada ao UAX #29 para o limite de 3.000 clusters e para a tarifação de 1 INK por cluster, incluindo espaços, pontuação e quebras de linha;
- uma dependência runtime adicional, confinada a um package puro e determinístico, com superfície de remoção mínima (uma função);
- atualização de versão segue um commit `build(deps)` isolado e exige reexecutar a suíte Unicode do owner;
- se a biblioteca padrão do Go passar a oferecer segmentação oficial, o owner é substituído sem tocar domínio ou aplicação.

## Evidências

- testes exploratórios em `internal/platform/text/graphemes_test.go` cobrindo ZWJ, combinantes, bandeiras, scripts pt/en, espaços/quebras e a fronteira de 3.000 clusters;
- `go.mod` fixa `github.com/rivo/uniseg v0.4.7` e `go.sum` registra os checksums;
- `DEPENDENCIES.md` atualizado com a versão, a licença e o owner; índice de ADRs atualizado.

## Revisão

Reavaliar quando a biblioteca padrão expuser segmentação de grapheme clusters, quando o owner deixar de ser o único consumidor ou na próxima revisão de dependências.
