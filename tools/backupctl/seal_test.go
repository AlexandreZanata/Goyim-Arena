package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testKey is a sealing key of the only size this format accepts.
func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, sealKeySize)
	for index := range key {
		key[index] = byte(index)
	}
	return key
}

func seal(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	var sealed bytes.Buffer
	writer, err := newSealedWriter(key, &sealed)
	if err != nil {
		t.Fatalf("newSealedWriter() error = %v", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return sealed.Bytes()
}

func unsealBytes(t *testing.T, key, sealed []byte) ([]byte, error) {
	t.Helper()
	var plaintext bytes.Buffer
	err := unseal(key, bytes.NewReader(sealed), &plaintext)
	return plaintext.Bytes(), err
}

// TestSealedRoundTripIsExactAtEveryChunkBoundary walks the sizes where a
// streaming format breaks: empty input, one byte, exactly one chunk, and the
// two sides of a chunk boundary. A format that only works for sizes it was
// tried with is a format that fails on the one backup that mattered.
func TestSealedRoundTripIsExactAtEveryChunkBoundary(t *testing.T) {
	t.Parallel()

	// The marker is what makes "the plaintext is not visible" a measurement
	// rather than a coincidence: a single random byte appears in a nonce about
	// four times in a thousand, and a check against one would fail for a reason
	// that has nothing to do with the cipher.
	marker := []byte("GAEB-MARKER-the-bytes-that-must-not-survive-a-seal-verbatim-0123456789")
	sizes := []int{0, 1, 4095, sealChunkSize - 1, sealChunkSize, sealChunkSize + 1, 3*sealChunkSize + 17}
	for _, size := range sizes {
		plaintext := make([]byte, size)
		if _, err := rand.Read(plaintext); err != nil {
			t.Fatalf("rand.Read() error = %v", err)
		}
		markerLength := min(len(marker), size)
		copy(plaintext, marker[:markerLength])

		sealed := seal(t, testKey(t), plaintext)
		recovered, err := unsealBytes(t, testKey(t), sealed)
		if err != nil {
			t.Fatalf("size %d: unseal() error = %v", size, err)
		}
		if !bytes.Equal(recovered, plaintext) {
			t.Fatalf("size %d: the unsealed bytes are not the sealed ones", size)
		}
		if markerLength == len(marker) && bytes.Contains(sealed, marker) {
			t.Fatalf("size %d: the plaintext is visible in the sealed object", size)
		}
	}
}

// TestSealingIsRandomised is the property that makes the sealing worth having
// beyond confidentiality: the same database dumped twice does not produce the
// same object, so the store cannot be asked whether two backups are identical.
func TestSealingIsRandomised(t *testing.T) {
	t.Parallel()

	plaintext := []byte("the same snapshot of the same ledger")
	first := seal(t, testKey(t), plaintext)
	second := seal(t, testKey(t), plaintext)
	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same input are byte-identical: the nonce is not random")
	}
}

// TestSealedObjectSaysWhatItIs asserts the header, because the reader decides
// what it is holding before it decides what to do with it.
func TestSealedObjectSaysWhatItIs(t *testing.T) {
	t.Parallel()

	sealed := seal(t, testKey(t), []byte("payload"))
	if !bytes.HasPrefix(sealed, []byte(sealMagic)) {
		t.Fatalf("sealed object does not start with its magic: %q", sealed[:len(sealMagic)])
	}
	declared := binary.BigEndian.Uint32(sealed[len(sealMagic)+sealNonceSize : len(sealMagic)+sealNonceSize+4])
	if declared != sealChunkSize {
		t.Fatalf("sealed object declares chunk size %d, want %d", declared, sealChunkSize)
	}
	if !bytes.HasSuffix(sealed, make([]byte, 4)) {
		t.Fatal("sealed object does not end with its end mark")
	}
}

// TestUnsealRefusesEveryEdit is the falsification half of the format: an object
// that was edited, cut short, reordered or replayed must fail, and it must fail
// with an error rather than with plausible bytes.
func TestUnsealRefusesEveryEdit(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 3*sealChunkSize)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	intact := seal(t, testKey(t), payload)

	cases := []struct {
		name   string
		sealed []byte
		want   string
	}{
		{
			name: "a byte flipped inside a record",
			sealed: func() []byte {
				edited := append([]byte(nil), intact...)
				edited[len(edited)/2] ^= 0x01
				return edited
			}(),
			want: "does not authenticate",
		},
		{
			name:   "an object cut before its end mark",
			sealed: append([]byte(nil), intact[:len(intact)-4]...),
			want:   "ends before its end mark",
		},
		{
			name:   "an object cut in the middle of a record",
			sealed: append([]byte(nil), intact[:len(intact)-100]...),
			want:   "ends before its end mark",
		},
		{
			name:   "bytes appended after the end mark",
			sealed: append(append([]byte(nil), intact...), 0x00, 0x01),
			want:   "after its end mark",
		},
		{
			name:   "an empty object",
			sealed: nil,
			want:   "not a sealed backup object",
		},
		{
			name:   "an object that is not sealed at all",
			sealed: bytes.Repeat([]byte("pg_dump"), 100),
			want:   "not a sealed backup object",
		},
		{
			name:   "the two records of a stream swapped",
			sealed: swapRecords(t, intact, sealChunkSize),
			want:   "does not authenticate",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := unsealBytes(t, testKey(t), testCase.sealed)
			if err == nil {
				t.Fatal("unseal() accepted an object it must refuse")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unseal() error = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// swapRecords exchanges the first two sealed records of a stream, which is the
// edit a reader without per-record associated data would not notice.
func swapRecords(t *testing.T, sealed []byte, chunkSize int) []byte {
	t.Helper()

	offset := len(sealMagic) + sealNonceSize + 4
	firstStart := offset
	firstLength := int(binary.BigEndian.Uint32(sealed[offset : offset+4]))
	firstEnd := offset + 4 + firstLength + 16
	secondStart := firstEnd
	secondLength := int(binary.BigEndian.Uint32(sealed[secondStart : secondStart+4]))
	secondEnd := secondStart + 4 + secondLength + 16
	if firstLength != chunkSize || secondLength != chunkSize {
		t.Fatalf("the fixture is not two full records (%d, %d)", firstLength, secondLength)
	}

	reordered := append([]byte(nil), sealed[:firstStart]...)
	reordered = append(reordered, sealed[secondStart:secondEnd]...)
	reordered = append(reordered, sealed[firstStart:firstEnd]...)
	reordered = append(reordered, sealed[secondEnd:]...)
	return reordered
}

// TestUnsealRefusesAnotherKey is the one failure an operator is most likely to
// meet, and the message has to point at the key rather than at the store.
func TestUnsealRefusesAnotherKey(t *testing.T) {
	t.Parallel()

	sealed := seal(t, testKey(t), []byte("a snapshot sealed with the old key"))
	other := testKey(t)
	other[0] ^= 0xff

	_, err := unsealBytes(t, other, sealed)
	if err == nil {
		t.Fatal("unseal() accepted an object sealed with a different key")
	}
	if !strings.Contains(err.Error(), "does not authenticate") {
		t.Fatalf("unseal() error = %v, want an authentication failure", err)
	}
}

// TestKeyFileRules covers the key material's own contract: a generated key is
// 0600 and reusable, and every shape that is not a 256-bit key is refused
// rather than stretched or truncated.
func TestKeyFileRules(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "backup.key")
	if err := GenerateKey(path); err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("generated key mode = %04o, want 0600", info.Mode().Perm())
	}
	key, err := LoadKey(path)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}
	if len(key) != sealKeySize {
		t.Fatalf("LoadKey() returned %d bytes, want %d", len(key), sealKeySize)
	}
	if err := GenerateKey(path); err == nil {
		t.Fatal("GenerateKey() replaced an existing key: every object sealed with it would become unreadable")
	}

	readable := filepath.Join(directory, "readable.key")
	if err := os.WriteFile(readable, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadKey(readable); err == nil || !strings.Contains(err.Error(), "readable beyond its owner") {
		t.Fatalf("LoadKey() accepted a group-readable key: %v", err)
	}

	short := filepath.Join(directory, "short.key")
	if err := os.WriteFile(short, []byte(base64.StdEncoding.EncodeToString(key[:16])+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadKey(short); err == nil || !strings.Contains(err.Error(), "decodes to 16 bytes") {
		t.Fatalf("LoadKey() accepted a 128-bit key: %v", err)
	}

	notBase64 := filepath.Join(directory, "text.key")
	if err := os.WriteFile(notBase64, []byte("not base64 at all!!\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadKey(notBase64); err == nil || !strings.Contains(err.Error(), "not base64") {
		t.Fatalf("LoadKey() accepted a file that is not base64: %v", err)
	}

	if _, err := LoadKey(filepath.Join(directory, "absent.key")); err == nil {
		t.Fatal("LoadKey() accepted a missing key file")
	}

	empty := filepath.Join(directory, "empty.key")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadKey(empty); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("LoadKey() accepted an empty key file: %v", err)
	}
}

// TestSealingRefusesAWeakKey closes the door the rest of the file leaves open:
// a key of the wrong size reaching the cipher.
func TestSealingRefusesAWeakKey(t *testing.T) {
	t.Parallel()

	if _, err := newSealedWriter([]byte("too short"), &bytes.Buffer{}); err == nil {
		t.Fatal("newSealedWriter() accepted a key that is not 256 bits")
	}
	if err := unseal([]byte("too short"), bytes.NewReader(nil), &bytes.Buffer{}); err == nil {
		t.Fatal("unseal() accepted a key that is not 256 bits")
	}
}

// TestUnsealRefusesAStreamThatLiesAboutItsChunkSize covers the header field a
// reader could be tempted to honour: the chunk size is part of the format, and
// a stream that declares another one is refused rather than believed.
func TestUnsealRefusesAStreamThatLiesAboutItsChunkSize(t *testing.T) {
	t.Parallel()

	sealed := seal(t, testKey(t), []byte("payload"))
	edited := append([]byte(nil), sealed...)
	binary.BigEndian.PutUint32(edited[len(sealMagic)+sealNonceSize:], 1<<20)

	_, err := unsealBytes(t, testKey(t), edited)
	if err == nil || !errors.Is(err, ErrSealedStream) {
		t.Fatalf("unseal() error = %v, want a refusal of the stream", err)
	}
}
