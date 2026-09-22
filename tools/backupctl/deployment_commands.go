package main

import (
	"fmt"
	"os"
)

// defaultComposeFile is the file this audit exists for. It is a constant rather
// than a parameter the caller may forget: the deployment is one file, and an
// audit that reads a different one is an audit of something else.
const defaultComposeFile = "compose.production.yaml"

// runCheckCompose reports what the committed Compose file breaks.
//
// It is the same audit the unit test runs, exposed as a command so that the
// behaviour gate (deploy/backup/verify.sh) can refuse a deployment whose
// database would not archive — one implementation, two places that ask it.
func runCheckCompose(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("%w: check-compose takes at most a path", errUsage)
	}
	path := defaultComposeFile
	if len(args) == 1 {
		path = args[0]
	}
	deployment, violations, err := AuditDeployment(path)
	if err != nil {
		return err
	}
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "backupctl: %s breaks %d archive rule(s):\n", path, len(violations))
		for _, violation := range violations {
			fmt.Fprintf(os.Stderr, "backupctl:   %s\n", violation)
		}
		return fmt.Errorf("the deployment does not archive: %d rule(s) broken", len(violations))
	}
	fmt.Printf("backupctl: %s declares the archive: %d flag(s) on the %s service, %d mount(s)\n",
		path, len(deployment.Command), deployment.Service, len(deployment.Volumes))
	return nil
}

// runPrintComposeCommand prints the database service's command, one argument
// per line.
//
// The gate uses it so that the server it starts during the exercise is the
// server the deployment declares: a second copy of the flags in the gate would
// be a copy that drifts from the file the audit reads.
func runPrintComposeCommand(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("%w: print-compose-command takes at most a path", errUsage)
	}
	path := defaultComposeFile
	if len(args) == 1 {
		path = args[0]
	}
	deployment, err := ReadDeployment(path)
	if err != nil {
		return err
	}
	if len(deployment.Command) == 0 {
		return fmt.Errorf("%s declares no command for the %s service", path, deployment.Service)
	}
	for _, argument := range deployment.Command {
		fmt.Println(argument)
	}
	return nil
}
