// The report half of the disaster drill (P20-T05).
//
// The phase asks for three things that have to be *registered* to count: the
// financial integrity, the measured RPO and RTO, and the load thresholds. A
// document that states them is a statement; a document whose numbers a gate
// refuses when they exceed the declared ceilings is a registration. So the run
// writes the facts it measured into one versioned document — prose for the
// reader, a fenced JSON block for this tool — and `check` applies the rules to
// the block, including the rule that the prose and the block are the same story.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The targets the release declares (docs/DEPLOYMENT.md §7): RPO of up to 15
// minutes and RTO of up to 4 hours, published as goals until exercises
// accumulate. They are constants here because the report's gate has to refuse a
// run that exceeds them, and a threshold that lives only in prose is a
// threshold nobody enforces.
const (
	targetRPOSeconds = 900
	targetRTOSeconds = 14400
)

// documentPath is the drill's own report.
const documentPath = "docs/DISASTER_DRILL.md"

// blockFence opens the machine-readable half of the report.
const blockFence = "```json"

// Facts is everything one drill measured. It is written by the exercise and read
// by both the renderer and the gate, so a number that is not here cannot appear
// in the report.
type Facts struct {
	// Version is the schema version of the block.
	Version int `json:"version"`
	// RunAt is the date of the run, YYYY-MM-DD.
	RunAt string `json:"run_at"`
	// Commit is the revision the exercise ran against.
	Commit string `json:"commit"`
	// Host describes the machine, because a baseline that does not name its
	// hardware is a baseline nobody can compare against.
	Host Host `json:"host"`
	// Dataset describes the synthetic data the drill seeded.
	Dataset Dataset `json:"dataset"`
	// Baseline is the financial state before the loss.
	Baseline Reading `json:"baseline"`
	// Restored is the financial state after the recovery.
	Restored Reading `json:"restored"`
	// Timeline pins the instants of the exercise.
	Timeline Timeline `json:"timeline"`
	// RPO is what could have been lost, bound and observed.
	RPO RPO `json:"rpo"`
	// RTO is how long the return took, against the declared target.
	RTO RTO `json:"rto"`
	// Outage is what the application did with a provider unavailable.
	Outage Outage `json:"outage"`
	// Load is the capacity baseline and the thresholds it was judged by.
	Load Load `json:"load"`
	// Journeys is what was driven against the restored data.
	Journeys Journeys `json:"journeys"`
	// Limitations is what the exercise found the delivered composition unable
	// to do, recorded where the report lists its own limits: a drill that hid
	// a missing surface would make the rest of its numbers easier to believe
	// than they deserve.
	Limitations []string `json:"limitations"`
	// Violations is what the comparison refused. Empty is the assertion.
	Violations []Violation `json:"violations"`
}

// Host is the machine a baseline belongs to.
type Host struct {
	Kernel string `json:"kernel"`
	CPUs   string `json:"cpus"`
	Docker string `json:"docker"`
}

// Dataset is the synthetic state the drill started from.
type Dataset struct {
	Seed     string `json:"seed"`
	Accounts int64  `json:"accounts"`
	INK      int64  `json:"ink"`
	Arena    string `json:"arena"`
}

// Timeline pins the moments of the exercise.
type Timeline struct {
	// BaselineAt is when the pre-loss snapshot was taken.
	BaselineAt string `json:"baseline_at"`
	// TargetAt is the instant the restore was asked to reach.
	TargetAt string `json:"target_at"`
	// DisasterAt is when the primary was destroyed; the RTO clock starts here.
	DisasterAt string `json:"disaster_at"`
	// RowsAfterTarget is how many rows the drill committed after the target and
	// how many of them the restored cluster holds. It must be zero.
	RowsAfterTarget int64 `json:"rows_after_target"`
	// RestoreCommand is the operator command the drill drove, so the report
	// shows the path an incident uses and not a private one.
	RestoreCommand string `json:"restore_command"`
	// PrimaryDestroyed records that the loss was a loss and not a comparison
	// between two copies of the same cluster.
	PrimaryDestroyed bool `json:"primary_destroyed"`
}

// RPO is the recovery point, bound and observed.
type RPO struct {
	// BoundSeconds is the archive_timeout the server runs with, read from the
	// server.
	BoundSeconds int64 `json:"bound_seconds"`
	// ObservedSeconds is how much time passed between the newest commit that
	// came back and the instant the primary was destroyed.
	ObservedSeconds int64 `json:"observed_seconds"`
	// ArchiveLagSeconds is how stale the archive was before the disaster.
	ArchiveLagSeconds int64 `json:"archive_lag_seconds"`
	// NewestRecoveredCommit is the timestamp of that newest commit.
	NewestRecoveredCommit string `json:"newest_recovered_commit"`
}

// RTO is the recovery time, against the target the release declares.
type RTO struct {
	TargetSeconds int64 `json:"target_seconds"`
	// ToWritableSeconds is the loss to a restored server accepting writes.
	ToWritableSeconds int64 `json:"to_writable_seconds"`
	// ToAppSeconds is the loss to the application answering on the restored
	// data, which is the number the product lives with.
	ToAppSeconds int64 `json:"to_app_seconds"`
}

// Outage is the behaviour with a provider unavailable.
type Outage struct {
	Email  EmailOutage  `json:"email"`
	Stripe StripeOutage `json:"stripe"`
}

// EmailOutage is the delivery path with the provider unavailable.
type EmailOutage struct {
	// AttemptsBefore is how many delivery attempts were recorded while the
	// provider was unreachable.
	AttemptsBefore int64 `json:"attempts_before"`
	// ErrorCode is the code the worker recorded for the failed attempt.
	ErrorCode string `json:"error_code"`
	// JobRetained is the assertion that matters: a failed delivery is retried,
	// never dropped.
	JobRetained bool `json:"job_retained"`
	// DeliveredAfter is the other direction: once the provider answers, the
	// same message is delivered.
	DeliveredAfter     bool  `json:"delivered_after"`
	DeliveredInSeconds int64 `json:"delivered_in_seconds"`
}

// StripeOutage is the payment path with the provider unavailable.
type StripeOutage struct {
	// Routes is what the running server answered on the provider surface: the
	// webhook the provider calls and the checkout the product calls.
	Routes []Probe `json:"routes"`
	// Verdict states what the exercise found: `unavailable` (the surface exists
	// and the failure is clean), `refused` (the surface exists and refuses) or
	// `surface_absent` (the surface is not composed, which is a finding and not
	// a pass).
	Verdict string `json:"verdict"`
	// LedgerRowsCreated is how many financial rows appeared while the provider
	// was unreachable. It must be zero: a provider that cannot be reached may
	// not move INK.
	LedgerRowsCreated int64 `json:"ledger_rows_created"`
	// Owner and Plan carry the finding when the verdict is `surface_absent`.
	Owner string `json:"owner"`
	Plan  string `json:"plan"`
}

// Probe is one request the exercise made and what came back.
type Probe struct {
	What   string `json:"what"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Want   int    `json:"want"`
}

// Load is the capacity baseline.
type Load struct {
	Tool            string      `json:"tool"`
	Script          string      `json:"script"`
	Dataset         string      `json:"dataset"`
	DurationSeconds int64       `json:"duration_seconds"`
	Thresholds      []Threshold `json:"thresholds"`
	// Note says what the figures above are figures *of* when the composition
	// could not answer the workload's routes: a latency of a 404 is a real
	// measurement and a misleading baseline, and the table is where a reader
	// looks first.
	Note string `json:"note"`
	// Breaches is what k6 reported as crossed. Empty is the assertion.
	Breaches []string `json:"breaches"`
}

// Threshold is one declared bound and what the run measured against it.
type Threshold struct {
	// Metric is the k6 metric key, exactly as the script tags it.
	Metric string `json:"metric"`
	// Bound is the expression the script declares.
	Bound string `json:"bound"`
	// Measured is what the run observed for it.
	Measured string `json:"measured"`
}

// Journeys is what was driven against the restored data.
type Journeys struct {
	Smoke     []Probe `json:"smoke"`
	E2E       bool    `json:"e2e"`
	E2EDetail string  `json:"e2e_detail"`
}

// reportDocument is the report split into its two halves.
type reportDocument struct {
	Prose    string
	Facts    Facts
	Findings []Violation
}

// readReport reads the report and parses the machine-readable half.
func readReport(path string) (reportDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return reportDocument{}, err
	}
	text := string(raw)
	fence := strings.Index(text, blockFence)
	if fence < 0 {
		return reportDocument{}, fmt.Errorf("%s: no %s block", path, blockFence)
	}
	body := text[fence+len(blockFence):]
	end := strings.Index(body, "```")
	if end < 0 {
		return reportDocument{}, fmt.Errorf("%s: the %s block is never closed", path, blockFence)
	}
	var facts Facts
	decoder := json.NewDecoder(strings.NewReader(body[:end]))
	// A field this tool cannot judge is a claim it cannot judge, and an
	// unjudged claim in a drill report is exactly what this gate refuses.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&facts); err != nil {
		return reportDocument{}, fmt.Errorf("%s: the block is not readable: %w", path, err)
	}
	return reportDocument{Prose: text[:fence], Facts: facts}, nil
}

// Check applies every rule to the report and returns what it refused.
func Check(document reportDocument) []Violation {
	var violations []Violation
	rules := []func(reportDocument) []Violation{
		ruleFactsShape,
		ruleFinancialIntegrity,
		ruleLossDirection,
		ruleRPO,
		ruleRTO,
		ruleLoadRegistered,
		ruleLoadVersioned,
		ruleOutageEmail,
		ruleOutageStripe,
		ruleJourneys,
		ruleProse,
	}
	for _, rule := range rules {
		violations = append(violations, rule(document)...)
	}
	return violations
}

// ruleFactsShape refuses a report whose facts cannot be judged: no version, no
// date, no revision, no hardware.
func ruleFactsShape(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	refuse := func(rule, detail string) {
		violations = append(violations, Violation{Rule: rule, Detail: detail})
	}
	if facts.Version < 1 {
		refuse("facts-version", fmt.Sprintf("version %d is not a schema version", facts.Version))
	}
	if _, err := time.Parse("2006-01-02", facts.RunAt); err != nil {
		refuse("facts-date", fmt.Sprintf("run_at %q is not a YYYY-MM-DD date", facts.RunAt))
	}
	if strings.TrimSpace(facts.Commit) == "" {
		refuse("facts-commit", "the report names no revision, so the baseline belongs to no code")
	}
	if strings.TrimSpace(facts.Host.Kernel) == "" || strings.TrimSpace(facts.Host.CPUs) == "" {
		refuse("facts-host", "the report names no hardware, and a baseline without hardware cannot be compared with anything")
	}
	return violations
}

// ruleFinancialIntegrity is the minimum validation of the phase: the ledger and
// the projection came back equal, with nothing lost and nothing created. It
// refuses a run that compared nothing, too, because an empty comparison is the
// cheapest way to be green.
func ruleFinancialIntegrity(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	refuse := func(rule, detail string) {
		violations = append(violations, Violation{Rule: rule, Detail: detail})
	}
	if len(facts.Baseline.Ledger.Wallets) == 0 || facts.Baseline.Ledger.Transactions == 0 {
		refuse("financial-evidence", "the baseline holds no wallet or no ledger row: a comparison of nothing proves nothing")
	}
	if len(facts.Restored.Ledger.Wallets) == 0 {
		refuse("financial-evidence", "the restored cluster holds no wallet")
	}
	for _, violation := range facts.Violations {
		violations = append(violations, Violation{
			Rule: "financial-integrity:" + violation.Rule, Subject: violation.Subject, Detail: violation.Detail,
		})
	}
	return violations
}

// ruleLossDirection refuses a point-in-time exercise that cannot show the
// boundary: rows committed after the target have to be absent, and the loss has
// to have been a loss.
func ruleLossDirection(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	if facts.Timeline.RowsAfterTarget != 0 {
		violations = append(violations, Violation{
			Rule: "loss-direction",
			Detail: fmt.Sprintf("the restored cluster holds %d row(s) committed after the target: the recovery overshot its boundary",
				facts.Timeline.RowsAfterTarget),
		})
	}
	if !facts.Timeline.PrimaryDestroyed {
		violations = append(violations, Violation{
			Rule:   "loss-direction",
			Detail: "the primary was not destroyed, so the exercise compared two copies of the same cluster instead of recovering from a loss",
		})
	}
	if strings.TrimSpace(facts.Timeline.RestoreCommand) == "" {
		violations = append(violations, Violation{
			Rule: "loss-direction", Detail: "the report names no restore command, so it does not show the path an incident would use",
		})
	}
	return violations
}

// ruleRPO refuses a recovery point worse than the bound the server declares and
// worse than the target the release publishes.
func ruleRPO(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	if facts.RPO.BoundSeconds <= 0 {
		violations = append(violations, Violation{
			Rule: "rpo-bound", Detail: "the archive_timeout bound was not read from the server, so the observed RPO has nothing to be judged against",
		})
	} else if facts.RPO.ObservedSeconds > facts.RPO.BoundSeconds {
		violations = append(violations, Violation{
			Rule: "rpo-bound", Detail: fmt.Sprintf(
				"the drill lost %ds of commits and the server archives with archive_timeout=%ds: the archive is worse than the deployment declares",
				facts.RPO.ObservedSeconds, facts.RPO.BoundSeconds),
		})
	}
	if facts.RPO.ObservedSeconds > targetRPOSeconds {
		violations = append(violations, Violation{
			Rule: "rpo-target", Detail: fmt.Sprintf(
				"the drill lost %ds of commits and the declared target is %ds", facts.RPO.ObservedSeconds, targetRPOSeconds),
		})
	}
	return violations
}

// ruleRTO refuses a return slower than the target the release declares.
func ruleRTO(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	if facts.RTO.TargetSeconds != targetRTOSeconds {
		violations = append(violations, Violation{
			Rule: "rto-target", Detail: fmt.Sprintf(
				"the report judges itself against %ds and the declared target is %ds", facts.RTO.TargetSeconds, targetRTOSeconds),
		})
	}
	if facts.RTO.ToAppSeconds <= 0 {
		violations = append(violations, Violation{
			Rule: "rto-target", Detail: "the drill measured no time to the application serving on the restored data",
		})
	} else if facts.RTO.ToAppSeconds > facts.RTO.TargetSeconds {
		violations = append(violations, Violation{
			Rule: "rto-target", Detail: fmt.Sprintf(
				"the application served on the restored data after %ds and the declared target is %ds",
				facts.RTO.ToAppSeconds, facts.RTO.TargetSeconds),
		})
	}
	return violations
}

// ruleLoadRegistered is the "thresholds de carga registrados sem erro crítico"
// of the phase: every declared bound is registered with what it measured, and
// nothing crossed.
func ruleLoadRegistered(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	if len(facts.Load.Thresholds) == 0 {
		violations = append(violations, Violation{
			Rule: "load-registered", Detail: "no load threshold was registered, so the baseline is a number without a budget",
		})
	}
	for _, threshold := range facts.Load.Thresholds {
		if strings.TrimSpace(threshold.Metric) == "" || strings.TrimSpace(threshold.Bound) == "" {
			violations = append(violations, Violation{
				Rule: "load-registered", Detail: fmt.Sprintf("the threshold %q carries no bound", threshold.Metric),
			})
		}
		if strings.TrimSpace(threshold.Measured) == "" || !isMeasured(threshold.Measured) {
			violations = append(violations, Violation{
				Rule: "load-registered", Subject: threshold.Metric,
				Detail: fmt.Sprintf("the threshold is registered with %q, which is a budget nobody stood in front of", threshold.Measured),
			})
		}
	}
	for _, breach := range facts.Load.Breaches {
		violations = append(violations, Violation{Rule: "load-breach", Detail: breach})
	}
	if strings.TrimSpace(facts.Load.Tool) == "" || strings.TrimSpace(facts.Load.Script) == "" {
		violations = append(violations, Violation{
			Rule: "load-registered", Detail: "the report names neither the tool nor the script, so the baseline is not reproducible",
		})
	}
	return violations
}

// isMeasured reports whether a registered figure is a number. A placeholder that
// reads as a measurement — "unknown", "n/a", a dash — is the one shape of value
// that would let a report claim a budget it never measured.
func isMeasured(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	switch strings.ToLower(trimmed) {
	case "unknown", "n/a", "na", "none", "-", "null":
		return false
	}
	_, err := strconv.ParseFloat(trimmed, 64)
	return err == nil
}

// ruleLoadVersioned refuses a threshold that is not the one the versioned script
// declares: a number typed into the report by hand is a number that can drift
// from the workload, and the drift is invisible.
func ruleLoadVersioned(document reportDocument) []Violation {
	path := document.Facts.Load.Script
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := readWorkload(path)
	if err != nil {
		return []Violation{{Rule: "load-thresholds-versioned", Detail: fmt.Sprintf("the workload %q cannot be read: %v", path, err)}}
	}
	script := string(raw)
	var violations []Violation
	for _, threshold := range document.Facts.Load.Thresholds {
		if !strings.Contains(script, threshold.Metric) {
			violations = append(violations, Violation{
				Rule: "load-thresholds-versioned", Subject: threshold.Metric,
				Detail: fmt.Sprintf("the report registers a threshold the workload %q does not declare", path),
			})
		}
	}
	return violations
}

// ruleOutageEmail refuses an outage exercise without both directions: the
// attempt failed with something recorded, the job survived it, and the same
// message went out once the provider answered.
func ruleOutageEmail(document reportDocument) []Violation {
	email := document.Facts.Outage.Email
	var violations []Violation
	refuse := func(detail string) {
		violations = append(violations, Violation{Rule: "outage-email", Detail: detail})
	}
	if email.AttemptsBefore < 1 {
		refuse("the exercise recorded no delivery attempt while the provider was unreachable, so nothing was tested")
	}
	if strings.TrimSpace(email.ErrorCode) == "" {
		refuse("the failed attempt was recorded with no error code, so the operator would not know why the message did not leave")
	}
	if !email.JobRetained {
		refuse("the delivery job was not retained: a message lost to an outage is a message the product silently dropped")
	}
	if !email.DeliveredAfter {
		refuse("the message was never delivered after the provider answered, so the retry path is unproven")
	}
	return violations
}

// ruleOutageStripe refuses a payment outage exercise that does not state what
// the provider surface did and does not prove that money did not move.
func ruleOutageStripe(document reportDocument) []Violation {
	stripe := document.Facts.Outage.Stripe
	var violations []Violation
	refuse := func(detail string) {
		violations = append(violations, Violation{Rule: "outage-stripe", Detail: detail})
	}
	switch stripe.Verdict {
	case "unavailable", "refused", "surface_absent":
	default:
		refuse(fmt.Sprintf("verdict %q is outside the vocabulary: the exercise has to say what it found", stripe.Verdict))
	}
	if len(stripe.Routes) == 0 {
		refuse("the exercise probed no provider route, so nothing about the payment surface was observed")
	}
	if stripe.LedgerRowsCreated != 0 {
		refuse(fmt.Sprintf("%d financial row(s) appeared while the provider was unreachable", stripe.LedgerRowsCreated))
	}
	if stripe.Verdict == "surface_absent" {
		if strings.TrimSpace(stripe.Owner) == "" || strings.TrimSpace(stripe.Plan) == "" {
			refuse("a provider surface that is not composed is a finding: it needs an owner and the work that closes it")
		}
	}
	return violations
}

// ruleJourneys refuses an exercise whose smoke did not answer what it asked and
// whose journeys did not run.
func ruleJourneys(document reportDocument) []Violation {
	journeys := document.Facts.Journeys
	var violations []Violation
	if len(journeys.Smoke) == 0 {
		violations = append(violations, Violation{Rule: "journeys-smoke", Detail: "the application was never asked anything on the restored data"})
	}
	for _, probe := range journeys.Smoke {
		if probe.Status != probe.Want {
			violations = append(violations, Violation{
				Rule: "journeys-smoke", Subject: probe.Path,
				Detail: fmt.Sprintf("%s answered %d and the drill asked for %d on the restored data", probe.What, probe.Status, probe.Want),
			})
		}
	}
	if !journeys.E2E {
		violations = append(violations, Violation{Rule: "journeys-e2e", Detail: "the browser journeys did not run against the restored instance"})
	}
	if strings.TrimSpace(journeys.E2EDetail) == "" {
		violations = append(violations, Violation{Rule: "journeys-e2e", Detail: "the report does not say what the journeys established"})
	}
	return violations
}

// ruleProse refuses a report whose prose forgot what the block measured: the two
// halves of one document that disagree are worse than either alone.
func ruleProse(document reportDocument) []Violation {
	facts := document.Facts
	var violations []Violation
	wants := map[string]string{
		"the observed RPO":   fmt.Sprintf("%ds", facts.RPO.ObservedSeconds),
		"the RPO bound":      fmt.Sprintf("%ds", facts.RPO.BoundSeconds),
		"the RTO to the app": fmt.Sprintf("%ds", facts.RTO.ToAppSeconds),
		"the target":         fmt.Sprintf("%ds", facts.RTO.TargetSeconds),
		"the ledger rows":    fmt.Sprintf("%d", facts.Baseline.Ledger.Transactions),
	}
	for what, value := range wants {
		if !strings.Contains(document.Prose, value) {
			violations = append(violations, Violation{
				Rule: "prose", Detail: fmt.Sprintf("the prose never states %s (%s) that the block carries", what, value),
			})
		}
	}
	for _, probe := range facts.Journeys.Smoke {
		if probe.Path != "" && !strings.Contains(document.Prose, probe.Path) {
			violations = append(violations, Violation{
				Rule: "prose", Detail: fmt.Sprintf("the prose never mentions the smoke of %s", probe.Path),
			})
		}
	}
	return violations
}

// readWorkload reads the versioned workload the report names. The path is the
// repository-relative one the report carries, and it is resolved from the
// working directory and then from each of its parents: the gate is invoked from
// the tree and from a package directory by the tests, and a rule that silently
// refused the workload because of where it was run from would be judging the
// caller instead of the report.
func readWorkload(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		return raw, nil
	}
	directory, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	searched := []string{directory}
	for {
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
		searched = append(searched, directory)
		if raw, err := os.ReadFile(filepath.Join(directory, path)); err == nil {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("not found under %s", strings.Join(searched, ", "))
}

// Render writes the report: the prose the reader gets, then the block the gate
// reads, both from the same facts.
func Render(facts Facts) string {
	var out strings.Builder

	out.WriteString("# Exercício de desastre e carga (P20-T05)\n\n")
	out.WriteString("**Status:** evidência de um exercício, não uma garantia de capacidade. Os números abaixo foram medidos; o que não foi medido não está aqui.\n\n")
	out.WriteString(fmt.Sprintf("**Executado em:** %s · **Commit:** `%s` · **Máquina:** %s, %s CPU(s), Docker %s\n\n",
		facts.RunAt, facts.Commit, facts.Host.Kernel, facts.Host.CPUs, facts.Host.Docker))

	out.WriteString("## 1. O que o exercício fez\n\n")
	out.WriteString("O caminho é o de um incidente, não um caminho paralelo: os mesmos scripts que a operação usa (`deploy/backup/base-backup.sh`, `deploy/backup/restore.sh`) sobre um PostgreSQL descartável, com o arquivamento contínuo que o `compose.production.yaml` declara. O dataset é sintético (seed `" + facts.Dataset.Seed + "`, " + fmt.Sprintf("%d", facts.Dataset.Accounts) + " conta(s), " + fmt.Sprintf("%d", facts.Dataset.INK) + " INK creditado pelo próprio caso de uso da carteira).\n\n")
	out.WriteString(fmt.Sprintf("1. o estado financeiro foi fotografado às **%s** (linha de base);\n2. o alvo escolhido foi **%s**;\n3. depois do alvo o exercício escreveu mais dados, que o restore **não** pode trazer: %d linha(s) ficaram de fora por desenho;\n4. o primário e o diretório de dados foram **destruídos** às **%s** — o que sobrou foi o armazenamento;\n5. a recuperação rodou `%s`;\n6. a aplicação subiu contra o cluster restaurado e respondeu perguntas reais sobre os dados que voltaram.\n\n",
		facts.Timeline.BaselineAt, facts.Timeline.TargetAt, facts.Timeline.RowsAfterTarget, facts.Timeline.DisasterAt, facts.Timeline.RestoreCommand))

	out.WriteString("## 2. Integridade financeira\n\n")
	ledger := facts.Baseline.Ledger
	out.WriteString(fmt.Sprintf("O que voltou é o mesmo dinheiro: **%d transação(ões) de ledger** em %d operação(ões), %d conta(s) e %d carteira(s), com o ledger hasheando `%s` e a projeção `%s` antes e depois do desastre. Nada foi criado, nada foi perdido: as duas leituras — a de antes e a de depois — são idênticas, e é isso que a comparação afirma linha a linha.\n\n",
		ledger.Transactions, ledger.Operations, ledger.Accounts, len(ledger.Wallets), ledger.LedgerDigest, ledger.BalancesDigest))
	out.WriteString(fmt.Sprintf("Saldo total antes: FREE_INK=%d, PURCHASED_INK=%d. Depois: FREE_INK=%d, PURCHASED_INK=%d.\n\n",
		ledger.FreeINK, ledger.PurchasedINK, facts.Restored.Ledger.FreeINK, facts.Restored.Ledger.PurchasedINK))
	if len(facts.Violations) == 0 {
		out.WriteString("A comparação não recusou nada: cada carteira voltou com o saldo que tinha, a projeção continua igual ao que o ledger deriva (a invariante da migration 00008), nenhum saldo ficou negativo e o agregado continua sendo a soma das partes.\n\n")
	} else {
		out.WriteString("A comparação **recusou**:\n\n")
		for _, violation := range facts.Violations {
			out.WriteString(fmt.Sprintf("- `%s`: %s\n", violation.Rule, violation.String()))
		}
		out.WriteString("\n")
	}

	out.WriteString("## 3. RPO e RTO medidos\n\n")
	out.WriteString(fmt.Sprintf("**RPO observado: %ds** (o commit mais novo que voltou é de %s, e o primário foi destruído depois dele) contra o limite de `archive_timeout` que o próprio servidor declara, **%ds**. O atraso do arquivo no fim do exercício foi de %ds. A meta declarada da release é de %ds.\n\n",
		facts.RPO.ObservedSeconds, facts.RPO.NewestRecoveredCommit, facts.RPO.BoundSeconds, facts.RPO.ArchiveLagSeconds, targetRPOSeconds))
	out.WriteString(fmt.Sprintf("**RTO: %ds** da destruição até o servidor restaurado aceitar escrita, e **%ds** até a aplicação responder na superfície pública sobre os dados restaurados. A meta declarada é de %ds.\n\n",
		facts.RTO.ToWritableSeconds, facts.RTO.ToAppSeconds, facts.RTO.TargetSeconds))
	out.WriteString("Os dois números são do exercício e não prometem nada: uma recuperação real paga rede, disco e decisão humana, e o que este exercício mede é o piso do processo.\n\n")

	out.WriteString("## 4. Provedor indisponível\n\n")
	email := facts.Outage.Email
	out.WriteString(fmt.Sprintf("**Email.** O trabalho de entrega rodou com o provedor inalcançável: **%d tentativa(s)** registrada(s) com o código `%s`, o job **%s** na fila (uma mensagem perdida numa indisponibilidade é uma mensagem que o produto descartou em silêncio) e, quando o provedor voltou a responder, a mesma mensagem foi entregue em %ds.\n\n",
		email.AttemptsBefore, email.ErrorCode, retained(email.JobRetained), email.DeliveredInSeconds))

	stripe := facts.Outage.Stripe
	out.WriteString(fmt.Sprintf("**Stripe.** A superfície do provedor foi sondada e o veredito é `%s`, com **%d** linha(s) financeira(s) criada(s) enquanto o provedor estava inalcançável. Sondagens:\n\n", stripe.Verdict, stripe.LedgerRowsCreated))
	for _, probe := range stripe.Routes {
		out.WriteString(fmt.Sprintf("- %s `%s` → **%d** (esperado %d)\n", probe.Method, probe.Path, probe.Status, probe.Want))
	}
	if stripe.Verdict == "surface_absent" {
		out.WriteString(fmt.Sprintf("\nUma superfície que não está composta é um **achado**, e não uma aprovação: dono `%s`, trabalho seguinte `%s`.\n", stripe.Owner, stripe.Plan))
	}
	out.WriteString("\n")

	out.WriteString("## 5. Baseline de carga\n\n")
	out.WriteString(fmt.Sprintf("Workload versionado `%s` (dataset `%s`), %s, %ds. Limites declarados pelo próprio script e o que o exercício mediu:\n\n",
		facts.Load.Script, facts.Load.Dataset, facts.Load.Tool, facts.Load.DurationSeconds))
	out.WriteString("| Métrica | Limite declarado | Medido |\n| --- | --- | --- |\n")
	for _, threshold := range facts.Load.Thresholds {
		out.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", threshold.Metric, threshold.Bound, threshold.Measured))
	}
	out.WriteString("\n")
	if facts.Load.Note != "" {
		out.WriteString(fmt.Sprintf("**Nota:** %s\n\n", escapeLine(facts.Load.Note)))
	}
	if len(facts.Load.Breaches) == 0 {
		out.WriteString("Nenhum limite foi cruzado: o baseline está registrado sem erro crítico.\n\n")
	} else {
		out.WriteString("Limites cruzados:\n\n")
		for _, breach := range facts.Load.Breaches {
			out.WriteString(fmt.Sprintf("- %s\n", breach))
		}
		out.WriteString("\n")
	}

	out.WriteString("## 6. Jornadas contra os dados restaurados\n\n")
	out.WriteString("A aplicação foi subida contra o cluster **restaurado** e respondeu:\n\n")
	for _, probe := range facts.Journeys.Smoke {
		out.WriteString(fmt.Sprintf("- %s `%s` → **%d**\n", probe.Method, probe.Path, probe.Status))
	}
	out.WriteString(fmt.Sprintf("\nE2E: %s. %s\n\n", journeysRun(facts.Journeys.E2E), facts.Journeys.E2EDetail))

	out.WriteString("## 7. Limites registrados, não escondidos\n\n")
	for _, limitation := range facts.Limitations {
		out.WriteString(fmt.Sprintf("- %s\n", escapeLine(limitation)))
	}
	out.WriteString("- o exercício mede o **piso** do processo: máquina local, dataset sintético e nenhum tráfego concorrente;\n")
	out.WriteString("- a carga é o smoke versionado da P17-T06, não um teste de capacidade: os limites são modestos de propósito e não são promessa;\n")
	out.WriteString("- a indisponibilidade do email é injetada no adapter de entrega, que é a fronteira do provedor dentro do processo; o que ela prova é a política do worker (registrar, manter, tentar de novo), que não muda com o fornecedor;\n")
	out.WriteString("- o RPO observado é o do instante do desastre, não uma média: `archive_timeout` continua sendo o pior caso declarado;\n")
	out.WriteString("- nenhum limiar foi reduzido, nenhum teste pulado, nenhum snapshot aceito, nenhum retry mascarado, nenhum waiver, nenhum `--force`/`--admin`/`--no-verify`;\n")
	out.WriteString("- o drill é manual (`make disaster-drill`) e não está em `make verify`: ele exige Docker, k6 e navegador, e um gate de merge não é um release.\n\n")

	out.WriteString("## 8. Fatos medidos (bloco executável)\n\n")
	out.WriteString("O bloco abaixo é o que o portão lê. Ele é a forma mecânica do que este documento diz, e `make disaster-drill` recusa quando os dois discordam ou quando um número cruza um teto declarado.\n\n")
	out.WriteString(blockFence + "\n")
	block, err := json.MarshalIndent(facts, "", "  ")
	if err == nil {
		out.Write(block)
	}
	out.WriteString("\n```\n")
	return out.String()
}

// escapeLine keeps one measured limitation on one bullet: a line break inside a
// list item would silently drop the rest of what was found.
func escapeLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// retained renders the retention assertion in the reader's language.
func retained(value bool) string {
	if value {
		return "sobreviveu"
	}
	return "NÃO sobreviveu"
}

// journeysRun renders the journey assertion.
func journeysRun(value bool) string {
	if value {
		return "executado e verde"
	}
	return "NÃO executado"
}
