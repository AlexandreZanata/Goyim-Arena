// What the audit measured, in the form a person reads (P20-T03).
//
// The report is the deliverable half of the gate: the phase asks for a
// versioned report with times and the dataset, so the numbers are printed with
// what they were measured over — the version, the file, what the migration
// cost, whether it waited, what the snapshot weighed, what survived — and the
// rules are printed with what broke them.
package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// summarize prints the run the way an operator watches it.
func summarize(data *reportData, out io.Writer) {
	passed, failed := 0, 0
	for _, rule := range data.Rules {
		if rule.Passed {
			passed++
			continue
		}
		failed++
	}
	fmt.Fprintf(out, "migrationaudit: %d migration(s) walked, %d snapshot(s) upgraded, %d rule(s) checked (%d passed, %d failed)\n",
		data.Versions, len(data.Upgrades), len(data.Rules), passed, failed)
	fmt.Fprintf(out, "migrationaudit: dataset: %d table(s) seeded with one row each, %d row(s) in total\n",
		len(data.Dataset.Seeded), data.Dataset.Rows)
	fmt.Fprintf(out, "migrationaudit: the whole exercise took %s\n", data.Duration.Round(time.Millisecond))
}

// render writes the full report.
func render(data *reportData, out io.Writer) {
	write := func(format string, arguments ...any) {
		fmt.Fprintf(out, format, arguments...)
	}

	write("# Relatório da auditoria de migrations\n\n")
	write("Gerado por `tools/migrationaudit` no commit %s, em %s, em %s.\n\n",
		shortCommit(data.Commit), data.Started, data.Duration.Round(time.Millisecond))
	write("O exercício roda contra um PostgreSQL descartável: um banco construído do zero, um degrau que aplica cada migration com um leitor ativo, um snapshot copiado e rolado para o head em **cada** versão, uma migration que falha pela metade e a recuperação do banco em que ela falhou.\n\n")

	write("## 1. Banco vazio\n\n")
	write("A referência: a história inteira aplicada por `dbmigrate.Up` numa base vazia. É contra este banco que todo snapshot é comparado.\n\n")
	write("| Versões | Relações | Linhas |\n| --- | --- | --- |\n")
	write("| %d | %d | %d |\n\n", len(data.Virgin.Relations), len(data.Virgin.Relations), totalRows(data.Virgin))

	write("## 2. Degrau: uma migration por vez, com um leitor ativo\n\n")
	write("Enquanto cada migration é aplicada, outra sessão mantém `ACCESS SHARE` — o lock de um `SELECT` — em todas as tabelas do schema. \"Esperou\" significa que a migration não terminaria enquanto a aplicação estivesse lendo: é a estimativa de lock que o `docs/DEPLOYMENT.md` §6 exige para uma janela.\n\n")
	write("| Versão | Arquivo | Tempo | Esperou por leitor | Bloqueio observado | Tabelas semeadas |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, step := range data.Ladder {
		waited := "não"
		observed := "—"
		if step.Blocked {
			waited = "**sim**"
			observed = "`" + waitingList(step.Waiting) + "`"
		}
		write("| %d | `%s` | %s | %s | %s | %d |\n",
			step.Version, step.Name, step.AppliedIn.Round(time.Microsecond), waited, observed, len(step.Seeded))
	}
	write("\n")

	write("## 3. Snapshot e upgrade de cada versão\n\n")
	write("Depois de cada versão o banco foi copiado e a cópia rolou para o head com as migrations restantes. O resultado tem de ser **o mesmo banco** que o banco vazio produziu, sem perder linha, e sem perder coluna que a versão do snapshot já tinha.\n\n")
	write("| Snapshot da versão | Migrations restantes | Tempo | Mesmo schema? | Colunas perdidas | Linhas perdidas | História completa? |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, step := range data.Upgrades {
		write("| %d | %d | %s | %s | %s | %s | %s |\n",
			step.Version, step.Rest, step.UpgradedIn.Round(time.Millisecond),
			yesNo(step.FingerprintOK), list(step.Contractions), list(step.LostRows), yesNo(step.VersionTableOK))
	}
	write("\n")

	write("## 4. Falha simulada e recuperação\n\n")
	if data.Failure.Version == 0 {
		write("Nenhuma falha foi simulada.\n\n")
	} else {
		write("Na versão %d, uma migration injetada criou uma tabela, inseriu uma linha e então dividiu por zero. O runner tem de desfazer **tudo** — a tabela, a linha e o registro de versão — e o banco tem de continuar rolável para o head, porque `migrate down` não existe no runner: a volta operacional é o processo e o schema segue em frente.\n\n",
			data.Failure.Version)
		write("A migration injetada falhou num banco que ainda tinha migrations reais pendentes: %d delas foram aplicadas antes dela.\n\n",
			data.Failure.AppliedBeforeFailure)
		write("| Erro | Versão da falha registrada? | Sobras | Relações perdidas | Linhas perdidas | Migrations restantes na recuperação | Recuperou? |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		write("| `%s` | %s | %s | %s | %s | %d | %s |\n\n",
			oneLine(data.Failure.Error), yesNo(data.Failure.ProbeRecorded),
			list(data.Failure.Leftovers), list(data.Failure.Missing), list(data.Failure.RowsLost),
			data.Failure.RecoverApplied, yesNo(data.Failure.Recovered))
	}

	write("## 5. Dataset\n\n")
	write("O audit escreve uma linha por tabela que aceita `INSERT ... DEFAULT VALUES`, a cada versão, e verifica em cada upgrade que nenhuma linha desapareceu. Um `INSERT` que não conhece o domínio não é um dataset de produção: é a prova de que a linha **atravessa** a história.\n\n")
	write("| Tabelas semeadas | Linhas | Piso | Não semeadas |\n| --- | --- | --- | --- |\n")
	write("| %d | %d | %d | %d |\n\n", len(data.Dataset.Seeded), data.Dataset.Rows, data.Dataset.Floor, len(data.Dataset.Unseedable))
	if len(data.Dataset.Unseedable) > 0 {
		write("As tabelas que o seed ingênuo não consegue preencher, e por quê:\n\n")
		write("| Tabela | Motivo |\n| --- | --- |\n")
		for _, table := range sortedKeys(booleanKeys(data.Dataset.Unseedable)) {
			write("| `%s` | `%s` |\n", table, data.Dataset.Unseedable[table])
		}
		write("\n")
	}

	write("## 6. Regras\n\n")
	write("| Regra | Gravidade | Objeto | Resultado | Detalhe |\n| --- | --- | --- | --- | --- |\n")
	for _, rule := range data.Rules {
		verdict := "**passa**"
		if !rule.Passed {
			if rule.Severity == severityAdvisory {
				verdict = "**advertência**"
			} else {
				verdict = "**FALHA**"
			}
		}
		write("| `%s` | %s | %s | %s | %s |\n", rule.Name, rule.Severity, rule.Subject, verdict, escape(rule.Detail))
	}
	write("\n")

	write("## 6b. Substituições em vigor (o que a história derruba e recria no mesmo arquivo)\n\n")
	write("Um `DROP` seguido da recriação do mesmo objeto na mesma migration não é uma contração: o objeto continua lá, com outro corpo. A lista existe para que os 47 casos medidos sejam visíveis — e para que uma migration que derrube algo **sem** recriar apareça como falha do portão, e não como ruído nesta lista.\n\n")
	write("| Versão | Arquivo | Objeto |\n| --- | --- | --- |\n")
	for _, statement := range data.Replaced {
		write("| %d | `%s` | `%s` |\n", statement.Version, statement.Name, statement.Object)
	}
	write("\n")

	write("## 7. Ledgers\n\n")
	write("As exceções declaradas: cada uma foi medida primeiro e justificada depois, e o audit falha quando a realidade deixa de casar com elas em qualquer direção.\n\n")
	writeLedger(out, "Contrações (o que uma migration do histórico remove)", data.Ledger.Contractions)
	writeBlocking(out, data.Ledger.Blocking)
	writeLedger(out, "Cascades", data.Ledger.Cascades)
	writeLedger(out, "Contrações: o que a história remove", data.Ledger.Contractions)
	writeLedger(out, "Append-only para o runtime (INSERT + SELECT, nunca UPDATE)", data.Ledger.AppendOnly)
	writeLedger(out, "Somente leitura para o runtime (SELECT apenas)", data.Ledger.ReadOnly)
	writeLedger(out, "Tabelas das quais o runtime pode apagar", data.Ledger.Deletable)
	write("Piso do dataset: **%d** tabela(s).\n\n", data.Ledger.SeedFloor)

	if len(data.Findings) > 0 {
		write("## 8. Achados (o portão recusa)\n\n")
		for _, finding := range data.Findings {
			write("- %s\n", escape(finding))
		}
		write("\n")
	}

	if len(data.Advisories) > 0 {
		write("## 9. Advertências (registradas, não recusam)\n\n")
		write("Medições em que o schema é defensável e o achado é um trabalho seguinte. Estão aqui com o objeto exato e com o trabalho a que foram atribuídas — uma advertência sem dono é um achado que ninguém leva.\n\n")
		for _, advisory := range data.Advisories {
			write("- %s\n", escape(advisory))
		}
		write("\n")
		write("| Regra | Trabalho seguinte |\n| --- | --- |\n")
		for _, rule := range data.Rules {
			if rule.Severity != severityAdvisory || rule.Passed {
				continue
			}
			write("| `%s` | %s |\n", rule.Name, escape(advisoryRules[rule.Name]))
		}
		write("\n")
	}
}

func writeLedger(out io.Writer, title string, entries []ledgerEntry) {
	fmt.Fprintf(out, "**%s**\n\n", title)
	if len(entries) == 0 {
		fmt.Fprintf(out, "Nenhuma.\n\n")
		return
	}
	fmt.Fprintf(out, "| Chave | Motivo |\n| --- | --- |\n")
	for _, entry := range entries {
		fmt.Fprintf(out, "| `%s` | %s |\n", entry.Key, escape(entry.Reason))
	}
	fmt.Fprintf(out, "\n")
}

func writeBlocking(out io.Writer, entries []ledgerEntry) {
	writeLedger(out, "Migrations que esperam por um leitor ativo (precisam de janela)", entries)
}

func booleanKeys(values map[string]string) map[string]bool {
	keys := make(map[string]bool, len(values))
	for key := range values {
		keys[key] = true
	}
	return keys
}

func yesNo(value bool) string {
	if value {
		return "sim"
	}
	return "**não**"
}

func list(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	return "`" + strings.Join(values, "`, `") + "`"
}

func escape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	if commit == "" {
		return "(desconhecido)"
	}
	return commit
}
