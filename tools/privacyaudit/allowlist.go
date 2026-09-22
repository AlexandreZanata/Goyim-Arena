// The allowlist half of the privacy audit (P20-T06).
//
// The phase's minimum validation is "public exports validated against
// allowlist", and an allowlist that lives only in the prose of a document is a
// promise. This file reads what a surface can actually emit — the JSON tags of
// the Go file that builds the document — and compares it with the list the
// register declares, in both directions:
//
//   - a key the code can emit and the register does not declare fails, because
//     an undeclared field on an export is a field nobody reviewed;
//   - a key the register declares and the code cannot emit fails too, because
//     an allowlist that has drifted from the code is a list nobody is keeping.
//
// Then it checks the shape of every emitted name against the categories the
// policy says never appear. The check is on the *names* the code chose, which
// is what a reviewer can enforce mechanically; a field that carries a forbidden
// category and hides behind another name is what the human review of the
// categories is for, and the register says so.
package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// jsonTag matches one JSON tag's name, including the empty and "-" forms, so a
// key deliberately removed from the wire is visible to the scan instead of
// silently absent.
var jsonTag = regexp.MustCompile("`json:\"([^\",]*)[^\"]*\"`")

// forbiddenPublicTokens are the categories the policy says never appear in a
// *public* export (docs/PRIVACY.md §6): an address, a payment or provider
// identifier, a network or device signal, moderation evidence, anything about
// an account's money, and any secret.
var forbiddenPublicTokens = []string{
	"email", "ip", "user", "agent", "device", "moderation", "evidence",
	"provider", "webhook", "secret", "password", "token", "api", "key",
	"card", "cvc", "fingerprint", "customer", "payment", "invoice",
	"checkout", "transaction", "wallet", "balance", "billing", "subscription",
	"pass", "purchase", "refund", "account",
}

// forbiddenSubjectTokens are the categories the subject's own export may not
// carry either, even though it may carry the subject's data: another party's
// identifier, a payment provider's customer or webhook record, moderation
// evidence, a device signal or an address, and any secret. The subject's own
// email, locale, sessions and money history are admitted by the contract and
// are not here.
var forbiddenSubjectTokens = []string{
	"provider", "webhook", "moderation", "evidence", "device", "ip", "user",
	"agent", "secret", "password", "token", "api", "cvc", "fingerprint",
	"customer", "card",
}

// forbiddenTokens returns the categories that may never appear on a surface.
func forbiddenTokens(surface string) []string {
	if surface == surfaceSubject {
		return forbiddenSubjectTokens
	}
	return forbiddenPublicTokens
}

// scanJSONKeys returns the JSON keys a Go file can emit, sorted and unique,
// with the line of each one. Tags are read from the source text rather than
// from reflection on purpose: the public export's wire types are unexported to
// the adapter that owns them, and a scan that only saw the exported types would
// miss the document a client actually receives.
func scanJSONKeys(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, match := range jsonTag.FindAllStringSubmatch(string(raw), -1) {
		name := strings.TrimSpace(match[1])
		if name == "" || name == "-" {
			continue
		}
		seen[name] = true
	}
	keys := make([]string, 0, len(seen))
	for name := range seen {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys, nil
}

// forbiddenShape reports the forbidden token a key carries, if any. Matching is
// by token — the pieces the name is built from — so "ip_address" is refused
// while "participants_total" is not: a substring scan would refuse the second
// one for containing "ip", and a rule that cries wolf is a rule that gets
// waived.
func forbiddenShape(surface, key string) string {
	tokens := strings.Split(strings.ToLower(key), "_")
	for _, forbidden := range forbiddenTokens(surface) {
		if forbidden == strings.ToLower(key) {
			return forbidden
		}
		for _, token := range tokens {
			if token == forbidden {
				return forbidden
			}
		}
	}
	return ""
}

// allowlistProblems compares one surface with its declaration in both
// directions and checks the shape of every key the code can emit.
func allowlistProblems(root string, entry Allowlist, keys []string) []Violation {
	var violations []Violation
	refuse := func(rule, detail string) {
		violations = append(violations, Violation{
			Path: entry.Path, Rule: rule, Subject: entry.ID, Detail: detail,
		})
	}

	if !sorted(entry.Keys) {
		refuse("allowlist-order", "the declared keys are not sorted and unique, so two runs of the register are not comparable")
	}
	if len(entry.Keys) == 0 {
		refuse("allowlist-empty", "the declaration is empty: a scan of nothing proves nothing")
	}

	declared := map[string]bool{}
	for _, key := range entry.Keys {
		declared[key] = true
	}
	emitted := map[string]bool{}
	for _, key := range keys {
		emitted[key] = true
		if !declared[key] {
			refuse("allowlist-undeclared", fmt.Sprintf("the code can emit %q and the register does not declare it", key))
		}
		if token := forbiddenShape(entry.Surface, key); token != "" {
			refuse("allowlist-forbidden", fmt.Sprintf("the %s surface can emit %q, whose name carries %q, which the policy says never appears", entry.Surface, key, token))
		}
	}
	for _, key := range entry.Keys {
		if !emitted[key] {
			refuse("allowlist-unused", fmt.Sprintf("the register declares %q and %s cannot emit it", key, entry.Path))
		}
	}
	return violations
}
