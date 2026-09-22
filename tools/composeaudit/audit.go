// The audit of the production Compose topology (P19-T02).
//
// It answers questions about the *rendered* document — what `docker compose
// config --format json` prints, with every variable substituted, every
// environment file inlined and every shorthand expanded — because the file a
// reviewer reads and the configuration Compose actually creates are two
// different objects. A service can be spelled with a shorthand that renders to
// something else; a network can be named in a service and never declared; a
// port can be published by a service that was supposed to have none.
//
// The questions, and why each one is asked:
//
//	image_pinned                is every service identified by digest, so that
//	                            "deploy" is promoting an artifact rather than
//	                            resolving a tag again?
//	image_build                 does any service build? A host that builds what
//	                            it deploys cannot roll back by reference.
//	ingress_single              is there exactly one way in?
//	published_port_not_http     does anything outside the ingress's own HTTP and
//	                            HTTPS ports become reachable?
//	database_port_exposed       is the database port published anywhere? (Its
//	                            own rule, because "nobody publishes it" and
//	                            "it is not 5432" are read differently by a
//	                            person auditing a firewall.)
//	network_declared            is every network a service joins declared?
//	database_internal           is every network the database joins internal,
//	                            so the process holding the data has no route out?
//	ingress_database_isolation  does the public ingress share a network with the
//	                            database?
//	restart_declared            does every service declare how it comes back?
//	limits_declared             does every service declare a CPU and memory
//	                            limit, so one runaway process cannot take the
//	                            host with it?
//	healthcheck_present         can every service be probed from inside? The
//	                            application image is distroless and carries one
//	                            binary: nothing inside it can speak HTTP, so a
//	                            service running it may omit a healthcheck — and
//	                            then it must declare a stop grace period,
//	                            because a service that cannot be probed must at
//	                            least be able to stop without abandoning work.
//	                            The stack gate proves that service's health
//	                            from outside, through the ingress.
//	application_roles           does the application image run in at least two
//	                            roles, with distinct commands? The same artifact
//	                            asked two questions is the topology's claim.
//	application_environment     do those roles receive the variables production
//	                            refuses to boot without?
//	database_password_isolated  is the database password absent from the
//	                            processes that open the database through a DSN?
//	literal_credential          does the committed file name a secret value
//	                            instead of a path or an interpolation?
//
// The last one reads the file as committed, not the rendered document: by the
// time a document is rendered, every environment file has been inlined, so a
// literal in the file is indistinguishable from an operator's real environment
// there — and the rendered document is not something this tool may print.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Options configures one audit.
type Options struct {
	// File is the Compose file as it is committed.
	File string
	// Document is the path of the rendered document written by
	// `docker compose config --format json`.
	Document string
	// ApplicationImage is the reference of the application artifact, pinned by
	// digest, exactly as the deployment names it. It is an input rather than a
	// guess: the tool must not decide which image is the product.
	ApplicationImage string
}

// Violation is one broken rule, named as the rule that broke.
type Violation struct {
	Rule    string
	Service string
	Detail  string
}

func (violation Violation) String() string {
	if violation.Service == "" {
		return fmt.Sprintf("%s: %s", violation.Rule, violation.Detail)
	}
	return fmt.Sprintf("%s: service %q: %s", violation.Rule, violation.Service, violation.Detail)
}

// Report is the measurement. The report is always produced, even when rules
// are broken: a gate that only says "no" is a gate nobody can act on.
type Report struct {
	Project          string
	Services         []string
	Ingress          string
	Database         string
	ApplicationImage string
	PublishedPorts   []string
	Networks         []string
	Violations       []Violation
}

// document models the subset of the rendered configuration this audit reads.
// Everything else in the document is deliberately unread: a rule about a field
// nobody asked about is a rule that fails on an upgrade of Compose itself.
type document struct {
	Name     string             `json:"name"`
	Services map[string]service `json:"services"`
	Networks map[string]network `json:"networks"`
	Volumes  map[string]any     `json:"volumes"`
}

type service struct {
	Image           string             `json:"image"`
	Command         []string           `json:"command"`
	Build           map[string]any     `json:"build"`
	Ports           []port             `json:"ports"`
	Restart         string             `json:"restart"`
	Healthcheck     map[string]any     `json:"healthcheck"`
	Networks        map[string]any     `json:"networks"`
	Environment     map[string]*string `json:"environment"`
	Volumes         []volumeMount      `json:"volumes"`
	StopGracePeriod string             `json:"stop_grace_period"`
	Logging         *logging           `json:"logging"`
	Deploy          *deploy            `json:"deploy"`
}

type logging struct {
	Driver  string            `json:"driver"`
	Options map[string]string `json:"options"`
}

type port struct {
	Target    int    `json:"target"`
	Published string `json:"published"`
	Protocol  string `json:"protocol"`
}

type volumeMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type network struct {
	Name     string `json:"name"`
	Internal bool   `json:"internal"`
}

type deploy struct {
	Resources *resources `json:"resources"`
}

type resources struct {
	Limits *limits `json:"limits"`
}

type limits struct {
	CPUs   any `json:"cpus"`
	Memory any `json:"memory"`
}

// digestReference is a repository pinned by the digest of a manifest list.
var digestReference = regexp.MustCompile(`^[^\s@]+@sha256:[0-9a-f]{64}$`)

// httpPorts are the only container ports that may be published: the ingress's
// own two. Anything else would be a service reachable from outside the host.
var httpPorts = map[int]bool{80: true, 443: true}

// databasePort is the port the PostgreSQL image listens on. It is named in the
// report so an operator reading the output of a passing run sees what the gate
// checked, and not only what it refused.
const databasePort = 5432

// productionRequirements are the variables production refuses to boot without:
// the DSN, the payment credential, the email credential and the sender address
// (internal/platform/config) plus the cursor signing secret the participation
// journey requires to be mounted at all (internal/bootstrap).
var productionRequirements = []string{
	"ARENA_DATABASE_URL",
	"ARENA_CURSOR_SECRET",
	"ARENA_RESEND_API_KEY",
	"ARENA_EMAIL_FROM",
	"ARENA_STRIPE_SECRET_KEY",
}

// secretShapedKey matches a mapping key whose name promises a credential. The
// scan is textual and deliberately conservative: it reads only the file as
// committed, where an interpolation is visibly an interpolation.
var secretShapedKey = regexp.MustCompile(`(?i)^[A-Za-z0-9_.-]*(password|passwd|secret|token|api[_-]?key|private[_-]?key|credential|dsn)[A-Za-z0-9_.-]*$`)

// interpolation is a whole value that is a Compose variable reference.
var interpolation = regexp.MustCompile(`^\$\{[^}]+\}$`)

// Audit reads the two inputs and answers every question.
func Audit(options Options) (Report, error) {
	raw, err := os.ReadFile(options.Document)
	if err != nil {
		return Report{}, fmt.Errorf("read rendered document: %w", err)
	}
	var rendered document
	if err := json.Unmarshal(raw, &rendered); err != nil {
		return Report{}, fmt.Errorf("parse rendered document: %w", err)
	}
	if len(rendered.Services) == 0 {
		return Report{}, fmt.Errorf("the rendered document declares no service")
	}

	fileText, err := os.ReadFile(options.File)
	if err != nil {
		return Report{}, fmt.Errorf("read compose file: %w", err)
	}

	report := Report{
		Project:          rendered.Name,
		ApplicationImage: options.ApplicationImage,
		Services:         sortedKeys(rendered.Services),
		Networks:         sortedKeys(rendered.Networks),
	}
	violations := &report.Violations

	// The two roles the audit has to find before it can ask anything else. They
	// are found by what they are, not by what they are called: the ingress is
	// the service that publishes a port, the database is the service that runs
	// a PostgreSQL image.
	ingresses := make([]string, 0, 1)
	databases := make([]string, 0, 1)
	applications := make([]string, 0, 2)
	for _, name := range report.Services {
		instance := rendered.Services[name]
		if len(instance.Ports) > 0 {
			ingresses = append(ingresses, name)
		}
		if isPostgreSQL(instance.Image) {
			databases = append(databases, name)
		}
		if instance.Image == options.ApplicationImage {
			applications = append(applications, name)
		}
	}

	report.Ingress = single(ingresses)
	report.Database = single(databases)
	if len(ingresses) != 1 {
		*violations = append(*violations, Violation{
			Rule:   "ingress_single",
			Detail: fmt.Sprintf("%d services publish a port (%s); exactly one way into the stack is what makes the rest of the audit meaningful", len(ingresses), strings.Join(ingresses, ", ")),
		})
	}
	if len(databases) != 1 {
		*violations = append(*violations, Violation{
			Rule:   "database_missing",
			Detail: fmt.Sprintf("%d services run a PostgreSQL image; the database this topology is written around is not there", len(databases)),
		})
	}
	if len(applications) < 2 {
		*violations = append(*violations, Violation{
			Rule:   "application_roles",
			Detail: fmt.Sprintf("the application image runs in %d service(s) (%s); it is expected in at least two roles — the process that serves and the process that executes queued work", len(applications), strings.Join(applications, ", ")),
		})
	}

	// Networks the audit compares against: declared ones, and the ones each
	// role joined.
	joined := func(name string) []string {
		names := make([]string, 0, len(rendered.Services[name].Networks))
		for networkName := range rendered.Services[name].Networks {
			names = append(names, networkName)
		}
		sort.Strings(names)
		return names
	}

	for _, name := range report.Services {
		instance := rendered.Services[name]

		// image_pinned / image_build
		if !digestReference.MatchString(instance.Image) {
			*violations = append(*violations, Violation{
				Rule:    "image_pinned",
				Service: name,
				Detail:  fmt.Sprintf("image %q is not pinned by digest; a tag can be repointed under a review", instance.Image),
			})
		}
		if len(instance.Build) > 0 {
			*violations = append(*violations, Violation{
				Rule:    "image_build",
				Service: name,
				Detail:  "declares a build; the host that runs the stack must not compile what it deploys",
			})
		}

		// published_port_not_http / database_port_exposed / ingress_single
		if len(instance.Ports) > 0 && report.Ingress != "" && name != report.Ingress {
			*violations = append(*violations, Violation{
				Rule:    "ingress_single",
				Service: name,
				Detail:  fmt.Sprintf("publishes a port while %q already does; the ingress is the single public surface", report.Ingress),
			})
		}
		for _, published := range instance.Ports {
			report.PublishedPorts = append(report.PublishedPorts, fmt.Sprintf("%s->%d", published.Published, published.Target))
			if published.Target == databasePort {
				*violations = append(*violations, Violation{
					Rule:    "database_port_exposed",
					Service: name,
					Detail:  fmt.Sprintf("publishes container port %d (host %s): the database port is reachable from outside", published.Target, published.Published),
				})
			}
			if !httpPorts[published.Target] {
				*violations = append(*violations, Violation{
					Rule:    "published_port_not_http",
					Service: name,
					Detail:  fmt.Sprintf("publishes container port %d (host %s); only the ingress's own HTTP and HTTPS ports are publishable", published.Target, published.Published),
				})
			}
		}

		// network_declared / database_internal / ingress_database_isolation
		for _, networkName := range joined(name) {
			declared, ok := rendered.Networks[networkName]
			if !ok {
				*violations = append(*violations, Violation{
					Rule:    "network_declared",
					Service: name,
					Detail:  fmt.Sprintf("joins network %q, which the document never declares", networkName),
				})
				continue
			}
			if name == report.Database && !declared.Internal {
				*violations = append(*violations, Violation{
					Rule:    "database_internal",
					Service: name,
					Detail:  fmt.Sprintf("network %q is not internal: the process holding the data keeps a route to the internet", networkName),
				})
			}
		}

		// volume_declared
		for _, mount := range instance.Volumes {
			if mount.Type != "volume" {
				continue
			}
			if _, ok := rendered.Volumes[mount.Source]; !ok {
				*violations = append(*violations, Violation{
					Rule:    "volume_declared",
					Service: name,
					Detail:  fmt.Sprintf("mounts volume %q, which the document never declares", mount.Source),
				})
			}
		}

		// restart_declared
		switch instance.Restart {
		case "always", "unless-stopped", "on-failure":
		default:
			*violations = append(*violations, Violation{
				Rule:    "restart_declared",
				Service: name,
				Detail:  fmt.Sprintf("restart is %q; a long-running service declares how it comes back", orNone(instance.Restart)),
			})
		}

		// limits_declared
		if instance.Deploy == nil || instance.Deploy.Resources == nil || instance.Deploy.Resources.Limits == nil ||
			isEmpty(instance.Deploy.Resources.Limits.CPUs) || isEmpty(instance.Deploy.Resources.Limits.Memory) {
			*violations = append(*violations, Violation{
				Rule:    "limits_declared",
				Service: name,
				Detail:  "declares no CPU and memory limit; one runaway process must not be able to take the host",
			})
		}

		// logging_bounded
		//
		// A container whose log the daemon writes without a bound is a container
		// that can fill the disk the database lives on. The check reads the
		// rendered options, so a max-size that only exists as an interpolation
		// inside the file is still counted as the value the daemon would use.
		if instance.Logging == nil || strings.TrimSpace(instance.Logging.Options["max-size"]) == "" {
			*violations = append(*violations, Violation{
				Rule:    "logging_bounded",
				Service: name,
				Detail:  "declares no logging max-size; an unbounded log grows until the disk the database writes to is full",
			})
		}

		// healthcheck_present
		if len(instance.Healthcheck) == 0 {
			if instance.Image == options.ApplicationImage {
				if instance.StopGracePeriod == "" {
					*violations = append(*violations, Violation{
						Rule:    "healthcheck_present",
						Service: name,
						Detail:  "runs the application image, which has nothing inside it to probe, and declares no stop grace period either",
					})
				} else if _, err := time.ParseDuration(instance.StopGracePeriod); err != nil {
					*violations = append(*violations, Violation{
						Rule:    "healthcheck_present",
						Service: name,
						Detail:  fmt.Sprintf("declares an unreadable stop grace period %q", instance.StopGracePeriod),
					})
				}
			} else {
				*violations = append(*violations, Violation{
					Rule:    "healthcheck_present",
					Service: name,
					Detail:  "declares no healthcheck; this image can execute one, so a service that cannot be probed is a service nobody notices dying",
				})
			}
		}
	}

	// ingress_database_isolation compares the two roles once both are known.
	if report.Ingress != "" && report.Database != "" {
		ingressNetworks := joined(report.Ingress)
		databaseNetworks := map[string]bool{}
		for _, networkName := range joined(report.Database) {
			databaseNetworks[networkName] = true
		}
		for _, networkName := range ingressNetworks {
			if databaseNetworks[networkName] {
				*violations = append(*violations, Violation{
					Rule:   "ingress_database_isolation",
					Detail: fmt.Sprintf("the ingress %q and the database %q share network %q: the public surface has a route to the data", report.Ingress, report.Database, networkName),
				})
			}
		}
	}

	// application_roles / application_environment / database_password_isolated
	commands := map[string]string{}
	for _, name := range applications {
		instance := rendered.Services[name]

		if len(instance.Command) == 0 {
			*violations = append(*violations, Violation{
				Rule:    "application_roles",
				Service: name,
				Detail:  "runs the application image without a command; the image declares an entrypoint, so the role would be the default one",
			})
		} else {
			roleCommand := strings.Join(instance.Command, " ")
			if other, repeated := commands[roleCommand]; repeated {
				*violations = append(*violations, Violation{
					Rule:    "application_roles",
					Service: name,
					Detail:  fmt.Sprintf("runs the same command as %q (%q); two services of one artifact are two roles", other, roleCommand),
				})
			}
			commands[roleCommand] = name
		}

		// egress_declared
		//
		// Every role of the application calls out — the payment provider and the
		// email provider are both reached from inside these processes — so a
		// role that joined only internal networks has nowhere to send a
		// delivery, and a topology that cannot deliver is not a running
		// production stack. The check is a property of the file, not of the host
		// running the gate, which is what makes it assertable on any machine.
		reachesOut := false
		for _, networkName := range joined(name) {
			if declared, ok := rendered.Networks[networkName]; ok && !declared.Internal {
				reachesOut = true
				break
			}
		}
		if !reachesOut {
			*violations = append(*violations, Violation{
				Rule:    "egress_declared",
				Service: name,
				Detail:  fmt.Sprintf("joins only internal networks (%s); a role that delivers a message or charges a card needs a route out", strings.Join(joined(name), ", ")),
			})
		}

		if value, ok := instance.Environment["ARENA_ENV"]; !ok || value == nil || *value != "production" {
			*violations = append(*violations, Violation{
				Rule:    "application_environment",
				Service: name,
				Detail:  fmt.Sprintf("ARENA_ENV is %s; the topology exists to run production", orNone(deref(instance.Environment["ARENA_ENV"]))),
			})
		}
		for _, variable := range productionRequirements {
			if value, ok := instance.Environment[variable]; !ok || value == nil || strings.TrimSpace(*value) == "" {
				*violations = append(*violations, Violation{
					Rule:    "application_environment",
					Service: name,
					Detail:  fmt.Sprintf("%s reaches the process empty; production refuses to boot without it", variable),
				})
			}
		}
		if _, ok := instance.Environment["POSTGRES_PASSWORD"]; ok {
			*violations = append(*violations, Violation{
				Rule:    "database_password_isolated",
				Service: name,
				Detail:  "receives POSTGRES_PASSWORD; the application opens the database through ARENA_DATABASE_URL, so the database password reaches a process that has no use for it",
			})
		}
	}

	// literal_credential reads the file as committed.
	*violations = append(*violations, scanForLiteralCredentials(string(fileText))...)

	return report, nil
}

// scanForLiteralCredentials finds a credential-shaped key given a literal value
// in the committed Compose file. Interpolations and empty values are not
// literals: one names where the value comes from, the other declares the key.
func scanForLiteralCredentials(text string) []Violation {
	var violations []Violation
	for index, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		key = strings.Trim(key, "\"'")
		if !secretShapedKey.MatchString(key) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || interpolation.MatchString(value) || strings.HasPrefix(value, "#") {
			continue
		}
		violations = append(violations, Violation{
			Rule:   "literal_credential",
			Detail: fmt.Sprintf("line %d assigns a literal value to %q; the file names where a secret comes from, never what it is", index+1, key),
		})
	}
	return violations
}

// isPostgreSQL reports whether a reference names a PostgreSQL image.
func isPostgreSQL(reference string) bool {
	repository, _, _ := strings.Cut(reference, "@")
	return strings.Contains(strings.ToLower(repository), "postgres")
}

func single(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return ""
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case float64:
		return typed == 0
	default:
		return false
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func orNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
