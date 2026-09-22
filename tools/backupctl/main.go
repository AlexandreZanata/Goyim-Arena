// Command backupctl is the object-store half of the PostgreSQL backup pipeline
// (P19-T04). It performs one operation per invocation, so that the shell scripts
// that own the schedule, the retention window and the restore target stay
// readable:
//
//	backupctl keygen <key-file>            draw a sealing key (never replaces one)
//	backupctl put <name> <file> [--if-absent]
//	backupctl get <name> <file> [--raw]    unseals what it downloads; --raw is
//	                                      the audit path, and writes the sealed
//	                                      bytes as the store holds them
//	backupctl unseal <sealed> <file>       opens a sealed file already on this
//	                                      host, after the caller has checked it
//	backupctl list [prefix]                prints "<key>\t<size>\t<modified>"
//	backupctl latest-base [field]          the newest complete base backup
//	backupctl delete <name>                the retention policy's only tool
//	backupctl prune [--prefix base/] [--keep-days N] [--keep-min M] [--apply]
//	backupctl wal-fetch <name> <file>      PostgreSQL's restore_command, for a
//	                                      segment or a history file
//
// Configuration comes from the environment and never from the command line: an
// access key in argv is visible to every process on the host for as long as the
// command runs, which is exactly the property a backup credential must not
// have.
//
//	BACKUP_S3_ENDPOINT          scheme://host[:port] of the S3-compatible store
//	BACKUP_S3_BUCKET            the bucket that holds the backups
//	BACKUP_S3_ACCESS_KEY        the credential's id
//	BACKUP_S3_SECRET_KEY        the credential's secret
//	BACKUP_S3_REGION            default us-east-1
//	BACKUP_S3_PREFIX            the one prefix this credential owns, default none
//	BACKUP_ENCRYPTION_KEY_FILE  the sealing key, base64 of 32 bytes, mode 0600
//
// Object layout, which the retention policy is written against:
//
//	<prefix>/base/<name>.tar.gz.enc    one base backup, sealed
//	<prefix>/base/<name>.json.enc      its manifest, sealed
//	<prefix>/wal/<segment>.enc         one WAL segment, sealed
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// exitUsage is returned for a malformed invocation. It is distinct from a
// failed operation so that a typo in a script is not mistaken for a backup
// that failed.
const exitUsage = 2

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(os.Stderr, "backupctl: %v\n\n%s", err, usage)
			os.Exit(exitUsage)
		}
		fmt.Fprintf(os.Stderr, "backupctl: %v\n", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("invalid invocation")

const usage = `backupctl manages the sealed objects of the PostgreSQL backup pipeline.

Usage:

  backupctl keygen <key-file>
  backupctl put <name> <file> [--if-absent]
  backupctl get <name> <file> [--raw]
  backupctl unseal <sealed-file> <file>
  backupctl list [prefix]
  backupctl latest-base [name|start_lsn|created_at]
  backupctl delete <name>
  backupctl prune [--prefix base/] [--keep-days N] [--keep-min M] [--apply]
  backupctl wal-fetch <name> <file>

The object store is configured through BACKUP_S3_* and the sealing key through
BACKUP_ENCRYPTION_KEY_FILE; see the package comment for the names.
`

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: a subcommand is required", errUsage)
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
		return nil
	case "keygen":
		return runKeygen(args[1:])
	case "put":
		return runPut(args[1:])
	case "get":
		return runGet(args[1:])
	case "unseal":
		return runUnseal(args[1:])
	case "list":
		return runList(args[1:])
	case "latest-base":
		return runLatestBase(args[1:])
	case "delete":
		return runDelete(args[1:])
	case "prune":
		return runPrune(args[1:])
	case "wal-fetch":
		return runWalFetch(args[1:])
	case "check-compose":
		return runCheckCompose(args[1:])
	case "print-compose-command":
		return runPrintComposeCommand(args[1:])
	default:
		return fmt.Errorf("%w: unknown subcommand %q", errUsage, args[0])
	}
}

func runKeygen(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: keygen takes the key file's path", errUsage)
	}
	if err := GenerateKey(args[0]); err != nil {
		return err
	}
	fmt.Printf("backupctl: wrote a new sealing key to %s (mode 0600, never replaced once it exists)\n", args[0])
	return nil
}

func runPut(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("%w: put takes a name, a file, and optionally --if-absent and --record <path>", errUsage)
	}
	ifAbsent := false
	record := ""
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--if-absent":
			ifAbsent = true
		case "--record":
			index++
			if index >= len(args) {
				return fmt.Errorf("%w: --record takes a path", errUsage)
			}
			record = args[index]
		default:
			return fmt.Errorf("%w: unknown flag %q", errUsage, args[index])
		}
	}

	key, err := sealingKey()
	if err != nil {
		return err
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}

	sealed, size, sum, err := sealFile(key, args[1])
	if err != nil {
		return err
	}
	defer os.Remove(sealed)

	source, err := os.Open(sealed)
	if err != nil {
		return fmt.Errorf("reopen sealed object: %w", err)
	}
	defer source.Close()

	err = store.Put(args[0], source, size, sum, ifAbsent)
	switch {
	case errors.Is(err, ErrPresent) && ifAbsent:
		// A WAL segment re-archived after a crash lands here, and landing here
		// is success: the name of a segment and its bytes are one thing for a
		// given timeline, so the store already holding it is the archive
		// already done.
		fmt.Printf("backupctl: %s is already archived (%d bytes); nothing to do\n", args[0], size)
		return nil
	case err != nil:
		return err
	}
	fmt.Printf("backupctl: uploaded %s (%d bytes sealed, sha256 %s)\n", args[0], size, hex.EncodeToString(sum[:]))
	if record != "" {
		// The record is what lets the caller write a manifest that quotes the
		// bytes the store actually holds, instead of a number it computed from
		// a second sealing pass that produced different bytes.
		document, err := json.Marshal(struct {
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{Bytes: size, SHA256: hex.EncodeToString(sum[:])})
		if err != nil {
			return fmt.Errorf("encode record: %w", err)
		}
		if err := os.WriteFile(record, append(document, '\n'), 0o600); err != nil {
			return fmt.Errorf("write record %s: %w", record, err)
		}
	}
	return nil
}

func runGet(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("%w: get takes a name, a destination file and optionally --raw", errUsage)
	}
	raw := false
	if len(args) == 3 {
		if args[2] != "--raw" {
			return fmt.Errorf("%w: unknown flag %q", errUsage, args[2])
		}
		raw = true
	}
	key, err := sealingKey()
	if err != nil {
		return err
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	destination, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer destination.Close()

	if raw {
		// --raw writes the bytes the store holds, sealed as they are. It is the
		// audit path: the question "is what left this host encrypted" can only
		// be answered by looking at what the store has, and a tool that always
		// unwrapped its answer would be answering a different question.
		digest := sha256.New()
		if err := store.Get(args[0], io.MultiWriter(destination, digest)); err != nil {
			os.Remove(args[1])
			return err
		}
		if err := destination.Sync(); err != nil {
			return fmt.Errorf("sync destination: %w", err)
		}
		info, err := destination.Stat()
		if err != nil {
			return fmt.Errorf("stat destination: %w", err)
		}
		fmt.Printf("backupctl: downloaded %s exactly as the store holds it (%d bytes sealed, sha256 %s)\n", args[0], info.Size(), hex.EncodeToString(digest.Sum(nil)))
		return nil
	}

	sealedSize, sealedSum, err := unsealFromStore(key, store, args[0], destination)
	if err != nil {
		// A download that failed leaves a partial file, and a partial file is
		// worse than none: the next reader would treat it as an object.
		os.Remove(args[1])
		return err
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("sync destination: %w", err)
	}
	// The sealed size and checksum are printed because they are what the
	// manifest of a base backup quotes: an operator comparing them is asking
	// the only question that distinguishes "the object arrived" from "the
	// object arrived unedited".
	fmt.Printf("backupctl: downloaded and unsealed %s (%d bytes sealed, sha256 %s)\n", args[0], sealedSize, hex.EncodeToString(sealedSum[:]))
	return nil
}

// unsealFromStore streams an object from the store and unseals it into a
// destination in one pass, returning the size and checksum of the *sealed*
// bytes it read.
//
// The pass is single because a base backup is large enough that buffering it
// would decide how much memory a restore needs, and the checksum is computed on
// the way past so that verifying it costs no second read.
func unsealFromStore(key []byte, store *Store, name string, destination io.Writer) (int64, [32]byte, error) {
	reader, writer := io.Pipe()
	download := make(chan error, 1)
	go func() {
		err := store.Get(name, writer)
		// A failed download is reported to the reader as a write error, so the
		// unsealing stops instead of waiting for bytes that will not come.
		_ = writer.CloseWithError(err)
		download <- err
	}()

	counting := &countingReader{source: reader, digest: sha256.New()}
	unsealErr := unseal(key, counting, destination)
	// Drain whatever the download still has to say, so that the goroutine is
	// not left holding the connection.
	_, _ = io.Copy(io.Discard, reader)
	reader.Close()
	downloadErr := <-download

	if downloadErr != nil {
		return 0, [32]byte{}, downloadErr
	}
	if unsealErr != nil {
		return 0, [32]byte{}, unsealErr
	}
	var sum [32]byte
	copy(sum[:], counting.digest.Sum(nil))
	return counting.written, sum, nil
}

// countingReader counts the sealed bytes it passes and keeps the checksum of
// them. The digest lives beside the counter because the two are read together
// and only together: a size without a checksum says nothing about the bytes.
type countingReader struct {
	source  io.Reader
	written int64
	digest  hash.Hash
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 {
		r.written += int64(n)
		_, _ = r.digest.Write(p[:n])
	}
	return n, err
}

// runUnseal opens a sealed file that is already on this host.
//
// It exists as a separate step because the restore path verifies the checksum
// quoted by the manifest against the *sealed* bytes before it unseals anything:
// a download that is unsealed first and checked afterwards has already written
// bytes it could not trust.
func runUnseal(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("%w: unseal takes a sealed file and a destination", errUsage)
	}
	key, err := sealingKey()
	if err != nil {
		return err
	}
	source, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("open sealed file: %w", err)
	}
	defer source.Close()
	destination, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer destination.Close()
	if err := unseal(key, source, destination); err != nil {
		os.Remove(args[1])
		return err
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("sync destination: %w", err)
	}
	info, err := destination.Stat()
	if err != nil {
		return fmt.Errorf("stat destination: %w", err)
	}
	fmt.Printf("backupctl: unsealed %s into %s (%d bytes)\n", args[0], args[1], info.Size())
	return nil
}

func runList(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("%w: list takes at most one prefix", errUsage)
	}
	prefix := ""
	if len(args) == 1 {
		prefix = args[0]
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	objects, err := store.List(prefix)
	if err != nil {
		return err
	}
	for _, object := range objects {
		modified := object.LastModified.Format(time.RFC3339)
		if object.LastModified.IsZero() {
			modified = "-"
		}
		fmt.Printf("%s\t%d\t%s\n", object.Key, object.Size, modified)
	}
	return nil
}

// runLatestBase prints the newest *complete* base backup: the one the restore
// path selects when it is not told which to use.
//
// Completeness is the manifest's presence, and that is why the selection lives
// here rather than in a shell glob: a run killed between the tar and the
// manifest leaves an object that a listing shows and a restore cannot use, and
// the newest such object would be exactly the wrong choice.
func runLatestBase(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("%w: latest-base takes at most the name of a manifest field", errUsage)
	}
	key, err := sealingKey()
	if err != nil {
		return err
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	manifests, err := readManifests(store, key)
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		return errors.New("no complete base backup is stored: the manifest of a backup is what a restore selects")
	}
	newest := manifests[0]
	for _, manifest := range manifests[1:] {
		if manifest.createdAt().After(newest.createdAt()) {
			newest = manifest
		}
	}
	if len(args) == 0 {
		fmt.Println(newest.Name)
		return nil
	}
	switch args[0] {
	case "name":
		fmt.Println(newest.Name)
	case "start_lsn":
		fmt.Println(newest.StartLSN)
	case "created_at":
		fmt.Println(newest.CreatedAt)
	default:
		return fmt.Errorf("%w: latest-base knows name, start_lsn and created_at", errUsage)
	}
	return nil
}

func runDelete(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: delete takes one name", errUsage)
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	if err := store.Delete(args[0]); err != nil {
		return err
	}
	fmt.Printf("backupctl: deleted %s\n", args[0])
	return nil
}

func runWalFetch(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("%w: wal-fetch takes a segment name and a destination", errUsage)
	}
	if !validArchivedName(args[0]) {
		return fmt.Errorf("%w: %q is not a name PostgreSQL archives", errUsage, args[0])
	}
	key, err := sealingKey()
	if err != nil {
		return err
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	destination, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer destination.Close()
	if _, _, err := unsealFromStore(key, store, walPrefix+args[0]+sealedSuffix, destination); err != nil {
		// PostgreSQL reads the command's exit status: a segment that is not
		// there yet must fail, because a zero status with an empty file would
		// be read as "this WAL segment is empty" and recovery would proceed
		// with a gap.
		os.Remove(args[1])
		return err
	}
	return destination.Sync()
}

// ---------------------------------------------------------------------------
// Retention
// ---------------------------------------------------------------------------

// basePrefix, walPrefix and sealedSuffix are the object layout, named once
// because the retention policy and the restore path must agree on it.
const (
	basePrefix   = "base/"
	walPrefix    = "wal/"
	sealedSuffix = ".enc"
)

// baseObjectSuffix is the pair of files one base backup owns.
var baseObjectSuffixes = []string{".tar.gz" + sealedSuffix, ".json" + sealedSuffix}

// baseManifest is what a base backup says about itself. It is written by the
// script that took the backup, because the figures come from the tool that took
// it (`pg_basebackup`), and read here because retention is decided from the
// oldest backup that stays.
type baseManifest struct {
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at"`
	StartLSN     string `json:"start_lsn"`
	Timeline     uint32 `json:"timeline"`
	Bytes        int64  `json:"bytes"`
	SHA256Sealed string `json:"sha256_sealed"`
}

func runPrune(args []string) error {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	prefix := flags.String("prefix", basePrefix, "the prefix to prune (base/ or wal/)")
	keepDays := flags.Int("keep-days", 30, "how many days of base backups to keep")
	keepMin := flags.Int("keep-min", 7, "how many base backups to keep regardless of age")
	apply := flags.Bool("apply", false, "delete what the policy selects (the default only reports)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%w: prune takes flags only", errUsage)
	}
	if *keepDays <= 0 || *keepMin <= 0 {
		return fmt.Errorf("%w: keep-days and keep-min must be positive", errUsage)
	}
	store, err := configuredStore()
	if err != nil {
		return err
	}
	key, err := sealingKey()
	if err != nil {
		return err
	}

	switch *prefix {
	case basePrefix:
		return pruneBase(store, key, *keepDays, *keepMin, *apply)
	case walPrefix:
		return pruneWAL(store, key, *keepDays, *keepMin, *apply)
	default:
		return fmt.Errorf("%w: prune knows %q and %q", errUsage, basePrefix, walPrefix)
	}
}

// selectBackups splits the base backups into the ones retention keeps and the
// ones it removes.
//
// The rule is deliberately two-sided: age decides, and a floor decides when age
// would remove too much. A retention window that is an age alone deletes the
// last backup of a deployment that stopped backing up — which is the deployment
// that needs it most — and a floor alone never frees any space.
func selectBackups(manifests []baseManifest, now time.Time, keepDays, keepMin int) (keep, remove []baseManifest) {
	sorted := append([]baseManifest(nil), manifests...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			earlier, later := sorted[j-1].createdAt(), sorted[j].createdAt()
			if !later.After(earlier) {
				break
			}
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	cutoff := now.AddDate(0, 0, -keepDays)
	for index, manifest := range sorted {
		recent := manifest.createdAt().After(cutoff)
		if index < keepMin || recent {
			keep = append(keep, manifest)
			continue
		}
		remove = append(remove, manifest)
	}
	return keep, remove
}

func (m baseManifest) createdAt() time.Time {
	parsed, err := time.Parse(time.RFC3339, m.CreatedAt)
	if err != nil {
		// A manifest whose date cannot be read is treated as the oldest thing
		// there is: the alternative is keeping an unknown object forever.
		return time.Time{}
	}
	return parsed.UTC()
}

func pruneBase(store *Store, key []byte, keepDays, keepMin int, apply bool) error {
	manifests, err := readManifests(store, key)
	if err != nil {
		return err
	}
	keep, remove := selectBackups(manifests, time.Now().UTC(), keepDays, keepMin)
	for _, manifest := range remove {
		for _, suffix := range baseObjectSuffixes {
			name := manifest.Name + suffix
			if apply {
				if err := store.Delete(basePrefix + name); err != nil {
					return err
				}
				fmt.Printf("backupctl: deleted %s\n", basePrefix+name)
				continue
			}
			fmt.Printf("backupctl: would delete %s\n", basePrefix+name)
		}
	}
	fmt.Printf("backupctl: %d base backup(s) kept, %d selected for removal (apply=%t)\n", len(keep), len(remove), apply)
	return nil
}

func pruneWAL(store *Store, key []byte, keepDays, keepMin int, apply bool) error {
	manifests, err := readManifests(store, key)
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		return errors.New("no base backup is stored: refusing to prune WAL, because without a base backup a WAL segment restores nothing")
	}
	keep, _ := selectBackups(manifests, time.Now().UTC(), keepDays, keepMin)
	floor, err := oldestSegmentOf(keep)
	if err != nil {
		return err
	}
	objects, err := store.List(walPrefix)
	if err != nil {
		return err
	}
	removed := 0
	for _, object := range objects {
		segment := strings.TrimSuffix(strings.TrimPrefix(object.Key, store.config.Prefix+"/"), sealedSuffix)
		segment = strings.TrimPrefix(segment, walPrefix)
		if !validSegmentName(segment) {
			// An object this policy cannot interpret is left alone: retention
			// deleting what it does not understand is how an operator loses
			// the object they needed.
			fmt.Fprintf(os.Stderr, "backupctl: leaving %s alone: it is not a WAL segment, and this policy removes only the names it can compare against the floor\n", object.Key)
			continue
		}
		if segment >= floor {
			continue
		}
		name := walPrefix + segment + sealedSuffix
		if apply {
			if err := store.Delete(name); err != nil {
				return err
			}
			fmt.Printf("backupctl: deleted %s (older than the floor %s)\n", name, floor)
			removed++
			continue
		}
		fmt.Printf("backupctl: would delete %s (older than the floor %s)\n", name, floor)
		removed++
	}
	fmt.Printf("backupctl: WAL floor is %s; %d segment(s) selected for removal (apply=%t)\n", floor, removed, apply)
	return nil
}

// oldestSegmentOf is the retention floor: the WAL segment the oldest base
// backup that stays needs in order to be restored. Everything before it cannot
// be used by any backup that is still stored.
func oldestSegmentOf(keep []baseManifest) (string, error) {
	if len(keep) == 0 {
		return "", errors.New("retention kept no base backup, so no floor can be derived")
	}
	oldest := keep[0]
	for _, manifest := range keep[1:] {
		if manifest.createdAt().Before(oldest.createdAt()) {
			oldest = manifest
		}
	}
	if oldest.StartLSN == "" {
		return "", fmt.Errorf("the oldest kept base backup %s has no start LSN, so its floor is unknown", oldest.Name)
	}
	if oldest.Timeline == 0 {
		return "", fmt.Errorf("the oldest kept base backup %s has no timeline", oldest.Name)
	}
	return segmentOfLSN(oldest.Timeline, oldest.StartLSN)
}

// segmentOfLSN converts a log sequence number into the WAL segment that
// contains it, which is the form the archive and the retention floor are named
// in: <timeline><log><segment>, eight hexadecimal digits each, with a segment
// holding 16 MiB.
func segmentOfLSN(timeline uint32, lsn string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(lsn), "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("%q is not a log sequence number", lsn)
	}
	log, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return "", fmt.Errorf("%q is not a log sequence number: %w", lsn, err)
	}
	offset, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return "", fmt.Errorf("%q is not a log sequence number: %w", lsn, err)
	}
	return fmt.Sprintf("%08X%08X%08X", timeline, log, offset>>24), nil
}

// validSegmentName accepts the canonical 24-hex-digit WAL file name, and
// nothing else: the retention policy compares segment names as strings, which
// is only meaningful when every name it compares has the same shape — and a
// string comparison against a name of a different shape would move the floor
// to a place nothing sits, which is how a kept backup loses its WAL.
func validSegmentName(name string) bool {
	return validHexRun(name, 24)
}

// validHexRun reports whether value is exactly count hexadecimal digits.
func validHexRun(value string, count int) bool {
	if len(value) != count {
		return false
	}
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'A' && character <= 'F':
		default:
			return false
		}
	}
	return true
}

// validArchivedName accepts every file name PostgreSQL hands to archive_command,
// which is more than the segment: the server archives the backup history file it
// writes when a base backup finishes (<segment>.<offset>.backup), the timeline
// history after a switch (<timeline>.history) — without which a recovery cannot
// follow the switch, and the .partial segment that archive_mode=always produces.
// The restore_command has to fetch all of them: one that could fetch only the
// segments would stop recovery at the moment one of the others is asked for,
// which is exactly the moment a point-in-time restore is trying to reach.
func validArchivedName(name string) bool {
	if validSegmentName(name) {
		return true
	}
	if timeline, ok := strings.CutSuffix(name, ".history"); ok {
		return validHexRun(timeline, 8)
	}
	if segment, ok := strings.CutSuffix(name, ".backup"); ok {
		base, offset, found := strings.Cut(segment, ".")
		return found && validHexRun(base, 24) && validHexRun(offset, 8)
	}
	if segment, ok := strings.CutSuffix(name, ".partial"); ok {
		return validSegmentName(segment)
	}
	return false
}

func readManifests(store *Store, key []byte) ([]baseManifest, error) {
	objects, err := store.List(basePrefix)
	if err != nil {
		return nil, err
	}
	manifests := []baseManifest{}
	for _, object := range objects {
		name := strings.TrimSuffix(strings.TrimPrefix(object.Key, store.config.Prefix+"/"), sealedSuffix)
		name = strings.TrimPrefix(name, basePrefix)
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var buffer bytes.Buffer
		if _, _, err := unsealFromStore(key, store, basePrefix+name+sealedSuffix, &buffer); err != nil {
			if errors.Is(err, ErrAbsent) {
				continue
			}
			return nil, err
		}
		manifest := baseManifest{}
		if err := json.Unmarshal(buffer.Bytes(), &manifest); err != nil {
			return nil, fmt.Errorf("manifest %s is not readable: %w", name, err)
		}
		if manifest.Name == "" {
			manifest.Name = strings.TrimSuffix(name, ".json")
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

func configuredStore() (*Store, error) {
	config, err := StoreConfigFromEnvironment()
	if err != nil {
		return nil, err
	}
	return NewStore(config), nil
}

func sealingKey() ([]byte, error) {
	path := os.Getenv("BACKUP_ENCRYPTION_KEY_FILE")
	if path == "" {
		return nil, errors.New("BACKUP_ENCRYPTION_KEY_FILE is not set: the objects are sealed with it and there is no default")
	}
	return LoadKey(path)
}

// sealFile seals a local file into a temporary file beside it and returns the
// sealed path, its size and its SHA-256.
//
// The checksum is of the *sealed* bytes, because those are what the store
// holds and what the manifest quotes: a checksum of the plaintext would have to
// be recomputed before it could be compared with anything the store reports.
func sealFile(key []byte, path string) (string, int64, [32]byte, error) {
	source, err := os.Open(path)
	if err != nil {
		return "", 0, [32]byte{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer source.Close()

	sealed, err := os.CreateTemp(filepath.Dir(path), ".sealed-*")
	if err != nil {
		return "", 0, [32]byte{}, fmt.Errorf("create sealed file: %w", err)
	}
	if err := sealed.Chmod(0o600); err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, fmt.Errorf("chmod sealed file: %w", err)
	}

	digest := sha256.New()
	writer, err := newSealedWriter(key, io.MultiWriter(sealed, digest))
	if err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, err
	}
	if _, err := io.Copy(writer, source); err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, fmt.Errorf("seal %s: %w", path, err)
	}
	if err := writer.Close(); err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, err
	}
	if err := sealed.Sync(); err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, fmt.Errorf("sync sealed file: %w", err)
	}
	info, err := sealed.Stat()
	if err != nil {
		sealed.Close()
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, fmt.Errorf("stat sealed file: %w", err)
	}
	if err := sealed.Close(); err != nil {
		os.Remove(sealed.Name())
		return "", 0, [32]byte{}, fmt.Errorf("close sealed file: %w", err)
	}
	var sum [32]byte
	copy(sum[:], digest.Sum(nil))
	return sealed.Name(), info.Size(), sum, nil
}
