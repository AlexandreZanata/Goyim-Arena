package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeStore is an S3-compatible endpoint small enough to read.
//
// It verifies the signature of every request it receives, rebuilding the
// canonical request the way the specification defines it. That check is the
// point of this fake: the store refuses a request whose signature it cannot
// reproduce, so a client that signs the wrong host, an unsorted query or a
// payload hash it never hashed is caught here rather than by a provider at
// three in the morning. The full proof that the signature is right is the gate
// against a real S3-compatible server; this one proves the parts can agree.
type fakeStore struct {
	t        *testing.T
	access   string
	secret   string
	region   string
	objects  map[string][]byte
	requests int
	pages    int
	pageSize int
	failWith *failure
}

type failure struct {
	status int
	code   string
}

func newFakeStore(t *testing.T) *fakeStore {
	t.Helper()
	return &fakeStore{
		t:        t,
		access:   "AKIAEXAMPLE",
		secret:   "secret-of-the-store",
		region:   "us-east-1",
		objects:  map[string][]byte{},
		pageSize: 1000,
	}
}

func (f *fakeStore) client(t *testing.T) *Store {
	t.Helper()
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	return NewStore(StoreConfig{
		Endpoint:  server.URL,
		Bucket:    "backups",
		Region:    f.region,
		AccessKey: f.access,
		SecretKey: f.secret,
		Client:    server.Client(),
	})
}

func (f *fakeStore) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	f.requests++
	if f.failWith != nil {
		f.writeError(writer, f.failWith.status, f.failWith.code, "injected by the test")
		return
	}
	if err := f.verifySignature(request); err != nil {
		f.writeError(writer, http.StatusForbidden, "SignatureDoesNotMatch", err.Error())
		return
	}

	bucket, key := splitPath(request.URL.Path)
	if bucket != "backups" {
		f.writeError(writer, http.StatusNotFound, "NoSuchBucket", bucket)
		return
	}

	if request.Method == http.MethodGet && key == "" {
		f.list(writer, request)
		return
	}

	switch request.Method {
	case http.MethodPut:
		body, err := io.ReadAll(request.Body)
		if err != nil {
			f.writeError(writer, http.StatusInternalServerError, "InternalError", err.Error())
			return
		}
		if declared := request.Header.Get("x-amz-content-sha256"); declared != hex.EncodeToString(sha256Sum(body)) {
			f.writeError(writer, http.StatusBadRequest, "XAmzContentSHA256Mismatch",
				fmt.Sprintf("declared %s, body hashes to %s", declared, hex.EncodeToString(sha256Sum(body))))
			return
		}
		f.objects[key] = body
		writer.WriteHeader(http.StatusOK)
	case http.MethodGet:
		body, ok := f.objects[key]
		if !ok {
			f.writeError(writer, http.StatusNotFound, "NoSuchKey", key)
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	case http.MethodHead:
		body, ok := f.objects[key]
		if !ok {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writer.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(f.objects, key)
		writer.WriteHeader(http.StatusNoContent)
	default:
		f.writeError(writer, http.StatusMethodNotAllowed, "MethodNotAllowed", request.Method)
	}
}

func (f *fakeStore) list(writer http.ResponseWriter, request *http.Request) {
	f.pages++
	query := request.URL.Query()
	if query.Get("list-type") != "2" {
		f.writeError(writer, http.StatusBadRequest, "InvalidArgument", "list-type must be 2")
		return
	}
	prefix := query.Get("prefix")
	keys := []string{}
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	start := 0
	if token := query.Get("continuation-token"); token != "" {
		index, err := strconv.Atoi(token)
		if err != nil {
			f.writeError(writer, http.StatusBadRequest, "InvalidArgument", "continuation-token")
			return
		}
		start = index
	}
	end := start + f.pageSize
	if end > len(keys) {
		end = len(keys)
	}

	document := struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Name        string   `xml:"Name"`
		Prefix      string   `xml:"Prefix"`
		IsTruncated bool     `xml:"IsTruncated"`
		Next        string   `xml:"NextContinuationToken,omitempty"`
		Contents    []struct {
			Key          string `xml:"Key"`
			Size         int64  `xml:"Size"`
			LastModified string `xml:"LastModified"`
		} `xml:"Contents"`
	}{Name: "backups", Prefix: prefix, IsTruncated: end < len(keys)}
	for _, key := range keys[start:end] {
		entry := struct {
			Key          string `xml:"Key"`
			Size         int64  `xml:"Size"`
			LastModified string `xml:"LastModified"`
		}{Key: key, Size: int64(len(f.objects[key])), LastModified: time.Unix(1700000000, 0).UTC().Format(time.RFC3339)}
		document.Contents = append(document.Contents, entry)
	}
	if document.IsTruncated {
		document.Next = strconv.Itoa(end)
	}
	body, err := xml.Marshal(document)
	if err != nil {
		f.writeError(writer, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(body)
}

// verifySignature rebuilds the canonical request and the string to sign, and
// compares the result with the Authorization header.
func (f *fakeStore) verifySignature(request *http.Request) error {
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 ") {
		return errors.New("no AWS4-HMAC-SHA256 authorization header")
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(authorization, "AWS4-HMAC-SHA256 "), ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) == 2 {
			fields[pair[0]] = pair[1]
		}
	}
	credential := strings.Split(fields["Credential"], "/")
	if len(credential) != 5 || credential[0] != f.access {
		return fmt.Errorf("credential %q is not this store's", fields["Credential"])
	}
	signedHeaders := strings.Split(fields["SignedHeaders"], ";")

	canonicalHeaders := strings.Builder{}
	for _, name := range signedHeaders {
		value := request.Header.Get(name)
		if name == "host" {
			value = request.Host
		}
		fmt.Fprintf(&canonicalHeaders, "%s:%s\n", name, value)
	}

	// The query is re-encoded from scratch, which is what catches a client that
	// signed a differently ordered or differently escaped query.
	query := strings.Builder{}
	names := []string{}
	values := request.URL.Query()
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for index, name := range names {
		if index > 0 {
			query.WriteString("&")
		}
		sorted := append([]string(nil), values[name]...)
		sort.Strings(sorted)
		for position, value := range sorted {
			if position > 0 {
				query.WriteString("&")
			}
			fmt.Fprintf(&query, "%s=%s", encodeKey(name), encodeKey(value))
		}
	}

	canonicalRequest := strings.Join([]string{
		request.Method,
		request.URL.EscapedPath(),
		query.String(),
		canonicalHeaders.String(),
		fields["SignedHeaders"],
		request.Header.Get("x-amz-content-sha256"),
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		request.Header.Get("x-amz-date"),
		strings.Join(credential[1:], "/"),
		hex.EncodeToString(sha256Sum([]byte(canonicalRequest))),
	}, "\n")

	expected := hmac.New(sha256.New, signingKey(f.secret, credential[1], credential[2]))
	expected.Write([]byte(stringToSign))
	if !hmac.Equal([]byte(hex.EncodeToString(expected.Sum(nil))), []byte(fields["Signature"])) {
		return fmt.Errorf("signature %s does not match %s", fields["Signature"], hex.EncodeToString(expected.Sum(nil)))
	}
	return nil
}

func (f *fakeStore) writeError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(status)
	fmt.Fprintf(writer, "<Error><Code>%s</Code><Message>%s</Message></Error>", code, message)
}

func splitPath(path string) (string, string) {
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// writeTemp writes a file the client can upload.
func writeTemp(t *testing.T, content []byte) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

// TestSignedRequestIsAcceptedByTheStore exercises every operation the pipeline
// uses against a store that verifies the signature: a client that signs
// anything wrongly fails here, before a backup depends on it.
func TestSignedRequestIsAcceptedByTheStore(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	store := fake.client(t)

	payload := []byte("the sealed bytes of one WAL segment")
	file := writeTemp(t, payload)
	sum := sha256.Sum256(payload)
	if err := store.Put("wal/000000010000000000000001.enc", file, int64(len(payload)), sum, false); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if stored := fake.objects["wal/000000010000000000000001.enc"]; !bytes.Equal(stored, payload) {
		t.Fatalf("the store holds %q, want the bytes that were uploaded", stored)
	}

	var downloaded bytes.Buffer
	if err := store.Get("wal/000000010000000000000001.enc", &downloaded); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(downloaded.Bytes(), payload) {
		t.Fatal("Get() returned different bytes than Put() uploaded")
	}

	size, exists, err := store.Head("wal/000000010000000000000001.enc")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if !exists || size != int64(len(payload)) {
		t.Fatalf("Head() = (%d, %t), want (%d, true)", size, exists, len(payload))
	}

	objects, err := store.List("wal/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(objects) != 1 || objects[0].Key != "wal/000000010000000000000001.enc" {
		t.Fatalf("List() = %+v, want the one uploaded object", objects)
	}
	if objects[0].LastModified.IsZero() {
		t.Fatal("List() did not read the modification time")
	}

	if err := store.Delete("wal/000000010000000000000001.enc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, exists, err := store.Head("wal/000000010000000000000001.enc"); err != nil || exists {
		t.Fatalf("Head() after Delete() = (%t, %v), want the object gone", exists, err)
	}
}

// TestPutRefusesToReplaceWhenAsked covers the WAL re-archive: the segment that
// is already stored is the archive already done, and a base backup must never
// be silently replaced.
func TestPutRefusesToReplaceWhenAsked(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	store := fake.client(t)

	first := []byte("first seal of the segment")
	file := writeTemp(t, first)
	sum := sha256.Sum256(first)
	if err := store.Put("wal/000000010000000000000009.enc", file, int64(len(first)), sum, true); err != nil {
		t.Fatalf("first Put() error = %v", err)
	}

	second := writeTemp(t, []byte("a different seal of the same segment"))
	otherSum := sha256.Sum256([]byte("a different seal of the same segment"))
	err := store.Put("wal/000000010000000000000009.enc", second, int64(len("a different seal of the same segment")), otherSum, true)
	if !errors.Is(err, ErrPresent) {
		t.Fatalf("second Put() error = %v, want ErrPresent", err)
	}
	if stored := fake.objects["wal/000000010000000000000009.enc"]; !bytes.Equal(stored, first) {
		t.Fatal("the store's object was replaced by a request that asked not to replace one")
	}
}

// TestAbsentObjectIsAValue is what lets the restore path tell "the segment is
// not archived yet" from "the credential is wrong".
func TestAbsentObjectIsAValue(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	store := fake.client(t)

	var buffer bytes.Buffer
	if err := store.Get("wal/0000000100000000000000FF.enc", &buffer); !errors.Is(err, ErrAbsent) {
		t.Fatalf("Get() error = %v, want ErrAbsent", err)
	}
	if _, exists, err := store.Head("wal/0000000100000000000000FF.enc"); err != nil || exists {
		t.Fatalf("Head() = (%t, %v), want (false, nil)", exists, err)
	}
}

// TestErrorDocumentIsReadIntoTheMessage: a bare status code sends an operator
// searching; the store's own code sends them to the policy.
func TestErrorDocumentIsReadIntoTheMessage(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	store := fake.client(t)
	fake.failWith = &failure{status: http.StatusForbidden, code: "AccessDenied"}

	err := store.Get("base/2026-09-21.json.enc", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("Get() error = %v, want the store's error code", err)
	}
}

// TestListFollowsContinuationTokens: a retention policy that stopped at the
// first page would silently keep everything, which is the failure that looks
// like success.
func TestListFollowsContinuationTokens(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	fake.pageSize = 2
	store := fake.client(t)
	for index := 0; index < 5; index++ {
		fake.objects[fmt.Sprintf("base/2026-09-0%d.tar.gz.enc", index+1)] = []byte("x")
	}
	objects, err := store.List("base/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(objects) != 5 {
		t.Fatalf("List() returned %d objects, want all 5 across the pages", len(objects))
	}
	if fake.pages != 3 {
		t.Fatalf("the store served %d page(s), want 3: the client did not follow the continuation token", fake.pages)
	}
	for index := 1; index < len(objects); index++ {
		if objects[index-1].Key >= objects[index].Key {
			t.Fatalf("List() returned %+v, want the keys in order", objects)
		}
	}
}

// TestStoreConfigurationNamesEveryMissingVariable: a deployment that has to be
// fixed twice for the same reason is a deployment that will be fixed twice.
func TestStoreConfigurationNamesEveryMissingVariable(t *testing.T) {
	for _, name := range []string{"BACKUP_S3_ENDPOINT", "BACKUP_S3_BUCKET", "BACKUP_S3_ACCESS_KEY", "BACKUP_S3_SECRET_KEY"} {
		t.Setenv(name, "")
	}
	_, err := StoreConfigFromEnvironment()
	if err == nil {
		t.Fatal("StoreConfigFromEnvironment() accepted an empty environment")
	}
	for _, name := range []string{"BACKUP_S3_ENDPOINT", "BACKUP_S3_BUCKET", "BACKUP_S3_ACCESS_KEY", "BACKUP_S3_SECRET_KEY"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("the error does not name %s: %v", name, err)
		}
	}

	t.Setenv("BACKUP_S3_ENDPOINT", "minio:9000")
	t.Setenv("BACKUP_S3_BUCKET", "backups")
	t.Setenv("BACKUP_S3_ACCESS_KEY", "key")
	t.Setenv("BACKUP_S3_SECRET_KEY", "secret")
	if _, err := StoreConfigFromEnvironment(); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("StoreConfigFromEnvironment() accepted an endpoint without a scheme: %v", err)
	}

	t.Setenv("BACKUP_S3_ENDPOINT", "http://minio:9000/")
	t.Setenv("BACKUP_S3_REGION", "")
	config, err := StoreConfigFromEnvironment()
	if err != nil {
		t.Fatalf("StoreConfigFromEnvironment() error = %v", err)
	}
	if config.Endpoint != "http://minio:9000" {
		t.Fatalf("Endpoint = %q, want the trailing slash removed", config.Endpoint)
	}
	if config.Region != "us-east-1" {
		t.Fatalf("Region = %q, want the default", config.Region)
	}
}

// TestObjectKeysArePrefixedOnce covers the prefix: the credential this process
// carries owns one prefix, and every operation must stay inside it.
func TestObjectKeysArePrefixedOnce(t *testing.T) {
	t.Parallel()

	fake := newFakeStore(t)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	store := NewStore(StoreConfig{
		Endpoint:  server.URL,
		Bucket:    "backups",
		Region:    "us-east-1",
		AccessKey: fake.access,
		SecretKey: fake.secret,
		Prefix:    "arena",
		Client:    server.Client(),
	})

	payload := []byte("sealed")
	file := writeTemp(t, payload)
	sum := sha256.Sum256(payload)
	if err := store.Put("wal/000000010000000000000001.enc", file, int64(len(payload)), sum, false); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, exists := fake.objects["arena/wal/000000010000000000000001.enc"]; !exists {
		t.Fatalf("the object landed at %v, want it under the configured prefix", keysOf(fake.objects))
	}
	objects, err := store.List("wal/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("List() = %+v, want the one object under the prefix", objects)
	}
}

func keysOf(objects map[string][]byte) []string {
	keys := make([]string, 0, len(objects))
	for key := range objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestPathStyleURLsAreBuiltFromTheEndpoint pins the addressing style: one
// endpoint string works against every S3-compatible store, which is the reason
// it was chosen over virtual-host style.
func TestPathStyleURLsAreBuiltFromTheEndpoint(t *testing.T) {
	t.Parallel()

	store := NewStore(StoreConfig{Endpoint: "https://s3.example.invalid", Bucket: "arena-backups", Prefix: "prod"})
	parsed, err := url.Parse(store.objectURL("base/2026-09-21.tar.gz.enc"))
	if err != nil {
		t.Fatalf("the object URL does not parse: %v", err)
	}
	if parsed.Host != "s3.example.invalid" || parsed.Path != "/arena-backups/prod/base/2026-09-21.tar.gz.enc" {
		t.Fatalf("object URL = %s, want the bucket and key in the path", parsed)
	}
}
