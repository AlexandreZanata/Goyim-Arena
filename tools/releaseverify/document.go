// Rendering the release checklist (P20-T07).
//
// The document has two halves, like every other register of this phase: prose
// for the reader and a fenced JSON block for the tools. The prose here is
// written from the facts and never by hand, so a number in a sentence and the
// same number in the block cannot disagree — and the reader who wants the
// measured value instead of the sentence has the block.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// documentPath is where the rendered checklist is committed.
const documentPath = "docs/RELEASE_CHECKLIST.md"

// registerFence opens the machine-readable block.
const registerFence = "```json"

// Render writes the document from the facts.
func Render(facts Facts) (string, error) {
	block, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("# Checklist de release do backend (P20-T07)\n\n")
	out.WriteString("Verificação reproduzível de ponta a ponta, rodada em uma árvore limpa no\n")
	fmt.Fprintf(&out, "commit `%s`, com cada comando da fase executado **duas vezes**. Este documento é\n", facts.Commit)
	out.WriteString("gerado por essa execução (`tools/releaseverify/verify.sh`): a prosa descreve o que\n")
	out.WriteString("foi medido e o bloco no fim carrega os números, para que a frase e a medição não\n")
	out.WriteString("possam divergir. Para rodar de novo: `make release-verify`.\n\n")

	out.WriteString("## 1. Veredito\n\n")
	if facts.Governance.Blocked {
		out.WriteString("**A verificação técnica passa; a release não está autorizada.** As duas coisas são\n")
		out.WriteString("independentes e o documento registra as duas: nada nesta execução falhou (todo\n")
		out.WriteString("comando passou em duas execuções, a árvore terminou limpa e `git fsck` não\n")
		out.WriteString("reportou erro), e ainda assim `make release-gate` recusa, porque há decisão do\n")
		out.WriteString("titular em aberto — e uma decisão não é um defeito que o código possa corrigir.\n\n")
		if len(facts.Governance.Open) > 0 {
			out.WriteString("Em aberto, nomeadas pelo próprio portão:\n\n")
			for _, open := range facts.Governance.Open {
				fmt.Fprintf(&out, "- %s\n", open)
			}
			out.WriteString("\n")
		}
	} else {
		out.WriteString("**A verificação técnica passa e o portão de release não recusa.**\n\n")
	}
	if len(facts.Governance.Pending) > 0 {
		out.WriteString("Decididas, com o passo de execução ainda pendente (o portão os lista e não são\n")
		out.WriteString("motivo de recusa):\n\n")
		for _, pending := range facts.Governance.Pending {
			fmt.Fprintf(&out, "- %s\n", pending)
		}
		out.WriteString("\n")
	}

	out.WriteString("## 2. Onde e com o quê\n\n")
	fmt.Fprintf(&out, "- **Árvore:** %s.\n", facts.Checkout.Kind)
	fmt.Fprintf(&out, "- **Commit:** `%s` na branch `%s`. O documento entra no commit seguinte: a árvore\n", facts.Commit, facts.Branch)
	out.WriteString("  verificada é a desse commit, e o único arquivo que muda depois dela é este.\n")
	if len(facts.Checkout.Overlay) > 0 {
		out.WriteString("\nOs arquivos desta tarefa, copiados sobre o commit e verificados junto com ele: a\n")
		out.WriteString("ferramenta da verificação é o instrumento, então ela entra no mesmo commit que o\n")
		out.WriteString("documento que produz.\n\n")
		out.WriteString("| Arquivo | Digest (sha256) |\n|---|---|\n")
		for _, entry := range facts.Checkout.Overlay {
			fmt.Fprintf(&out, "| `%s` | `%s` |\n", entry.Path, shortDigest(entry.SHA256))
		}
		out.WriteString("\n")
	}
	fmt.Fprintf(&out, "- **Máquina:** %s/%s, %d processador(es).\n", facts.Host.OS, facts.Host.Arch, facts.Host.CPUs)
	fmt.Fprintf(&out, "- **Toolchain:** %s, Node %s, npm %s, Docker %s, sqlc %s.\n",
		facts.Host.Go, facts.Host.Node, facts.Host.NPM, facts.Host.Docker, facts.Host.Sqlc)
	fmt.Fprintf(&out, "- **PostgreSQL:** %s.\n", facts.Host.Postgres)
	fmt.Fprintf(&out, "- **Origem do banco:** %s\n", databaseProvenance(facts))
	fmt.Fprintf(&out, "- **Repositório:** %d arquivo(s) rastreado(s); `git fsck` %s com %d erro(s).\n",
		facts.Repository.TrackedFiles, facts.Repository.Fsck, facts.Repository.FsckErrors)
	fmt.Fprintf(&out, "- **Diretório local:** %d arquivo(s) rastreado(s) (tem de ser zero: o plano local nunca\n  vai para o Git).\n\n",
		facts.Checkout.LocalTrackedFiles)

	out.WriteString("## 3. Dependências: somente lockfiles\n\n")
	out.WriteString("Nada é instalado por versão flutuante. Cada dependência vem do lockfile que o commit\n")
	out.WriteString("versiona, e o portão recusa o documento em que algum comando instale fora de um:\n\n")
	out.WriteString("| Lockfile | Digest (sha256) | Instalado por |\n|---|---|---|\n")
	for _, lock := range facts.Lockfiles {
		digest := lock.SHA256
		if len(digest) > 16 {
			digest = digest[:16]
		}
		fmt.Fprintf(&out, "| `%s` | `%s…` | `%s` |\n", lock.Path, digest, lock.Installs)
	}
	out.WriteString("\n")

	out.WriteString("## 4. Os comandos, duas vezes cada\n\n")
	out.WriteString("| Comando | Execuções | Tempo |\n|---|---|---|\n")
	for _, command := range facts.Commands {
		fmt.Fprintf(&out, "| `%s` | %d | %.1fs |\n", command.Command, len(command.Runs), command.TotalSeconds())
	}
	fmt.Fprintf(&out, "\nTempo somado dos comandos: **%.1fs**. Cada linha é o que a fase exige: o mesmo\n", totalSeconds(facts.Commands))
	out.WriteString("comando duas vezes, com o mesmo resultado.\n\n")

	out.WriteString("## 5. A imagem e o smoke\n\n")
	fmt.Fprintf(&out, "- **Imagem:** `%s` (`%s`, %.1f MiB).\n", facts.Image.Reference, facts.Image.Digest, float64(facts.Image.SizeBytes)/(1024*1024))
	fmt.Fprintf(&out, "- **Smoke:** `%s` — %s.\n\n", facts.Image.Smoke, facts.Image.SmokeDetail)

	out.WriteString("## 6. Limitações reais\n\n")
	out.WriteString("O que esta verificação **não** estabelece, declarado em vez de omitido:\n\n")
	for _, limit := range facts.Limits {
		fmt.Fprintf(&out, "- %s\n", limit)
	}
	if len(facts.Next) > 0 {
		out.WriteString("\n## 7. Depois daqui\n\n")
		for _, next := range facts.Next {
			fmt.Fprintf(&out, "- %s\n", next)
		}
	}

	out.WriteString("\n")
	out.WriteString(registerFence + "\n")
	out.WriteString(string(block))
	out.WriteString("\n```\n")
	return out.String(), nil
}

// databaseProvenance is where the server came from, taken from the block: the
// note of the command that measured it. The gates run against whatever answers
// on the port the harnesses look for — sometimes a container this run started,
// sometimes one that was already there — and the document says which instead of
// assuming the one that sounds better.
func databaseProvenance(facts Facts) string {
	for _, command := range facts.Commands {
		if command.Key == "database" {
			return command.Note
		}
	}
	return "o servidor que os harnesses procuram em 127.0.0.1:54329"
}

// shortDigest clips a digest for a table cell, leaving short values alone so a
// malformed one is visible instead of silently cut.
func shortDigest(value string) string {
	if len(value) <= 20 {
		return value
	}
	return value[:20] + "…"
}

// totalSeconds sums every command's runs.
func totalSeconds(commands []Command) float64 {
	total := 0.0
	for _, command := range commands {
		total += command.TotalSeconds()
	}
	return total
}

// Document is the rendered checklist split into prose and block.
type Document struct {
	// Path is where it was read from.
	Path string
	// Prose is everything before the machine-readable block.
	Prose string
	// Text is the whole document.
	Text string
	// Facts is the parsed block.
	Facts Facts
}

// ReadDocument reads the document and splits it into prose and block.
func ReadDocument(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := string(raw)
	fence := strings.Index(text, registerFence)
	if fence < 0 {
		return Document{}, fmt.Errorf("%s: no %s block", path, registerFence)
	}
	body := text[fence+len(registerFence):]
	end := strings.Index(body, "```")
	if end < 0 {
		return Document{}, fmt.Errorf("%s: the %s block is never closed", path, registerFence)
	}

	var facts Facts
	decoder := json.NewDecoder(bytes.NewReader([]byte(body[:end])))
	// A field the tool does not know is a measurement the tool cannot judge.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&facts); err != nil {
		return Document{}, fmt.Errorf("%s: the block is not readable: %w", path, err)
	}
	return Document{Path: path, Prose: text[:fence], Text: text, Facts: facts}, nil
}
