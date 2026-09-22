// Sealing of backup objects: the bytes that leave for the object store are
// never the bytes of the database (P19-T04).
//
// Why the encryption lives in Go and not in a command: the standard library
// already has an authenticated cipher, and a pipeline that shells out to
// `openssl enc` would put the key on a command line (visible in the process
// table for as long as the seal runs) and would not authenticate the stream.
// AES-256-GCM authenticates each record, so a truncated or edited object is
// refused rather than restored.
//
// The format, and why each part of it exists:
//
//	"GAEB1\n"                        magic and version: a sealed object says
//	                                 what it is, and a version bump is a new
//	                                 magic rather than a guess about the old one
//	12-byte base nonce               random per object, so two seals of the same
//	                                 input differ
//	uint32 chunk size                65536, the plaintext bytes per record
//	records, each:                   one chunk, sealed on its own
//	  uint32 plaintext length
//	  ciphertext + 16-byte tag
//	uint32 zero                      the end mark: a stream that stops before
//	                                 it was truncated, and truncation is
//	                                 refused instead of restored
//
// Every record's associated data is the base nonce followed by the record
// index, which is what makes reordering or replaying a record a decryption
// failure rather than a silent corruption.
//
// Nothing here holds a whole object in memory, because a base backup of a real
// database does not fit in one: the sealing is a stream in both directions.
package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// sealMagic identifies a sealed object and its format version.
const sealMagic = "GAEB1\n"

// sealChunkSize is the plaintext size of one sealed record. It is a constant
// rather than a per-object choice: a reader has to agree with every writer that
// ever produced an object, and "the chunk size is in the header" would invite a
// reader to honour a value a writer chose.
const sealChunkSize = 64 * 1024

// sealNonceSize is the base nonce and the length of its part of the associated
// data.
const sealNonceSize = 12

// sealKeySize is the AES-256 key length. A key of any other size is refused
// rather than stretched: a short key is a weak key, and a warning about it
// would be read once.
const sealKeySize = 32

// ErrSealedStream reports an object that is not a sealed stream, or is one this
// version cannot read. It is separate from a decryption failure because the two
// mean different things to an operator: "this is not our object" and "this
// object was edited".
var ErrSealedStream = errors.New("not a sealed backup object")

// LoadKey reads a 256-bit key from a file.
//
// The file holds the key base64-encoded, which is what makes it safe to move
// between a secret manager and a filesystem without a binary-editing step. The
// file's permissions are checked, because a key readable by the group is a key
// readable by whatever else runs on the host — the check is a refusal, not a
// warning, since a warning here is a warning nobody reads twice.
func LoadKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("backup key: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("backup key %s is readable beyond its owner (%04o); store it 0600", path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("backup key: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("backup key %s is empty", path)
	}
	key, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("backup key %s is not base64 (%w); it holds the base64 of %d random bytes", path, err, sealKeySize)
	}
	if len(key) != sealKeySize {
		return nil, fmt.Errorf("backup key %s decodes to %d bytes, want %d", path, len(key), sealKeySize)
	}
	return key, nil
}

// GenerateKey writes a fresh key to a path, refusing to replace an existing
// file: replacing a key silently is the same as losing every object sealed with
// it, and this is the one place where being destructive is easy to avoid.
func GenerateKey(path string) error {
	key := make([]byte, sealKeySize)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	// O_EXCL is the refusal: a key that already exists is a key an operator
	// meant to keep, and overwriting it would make every object sealed with it
	// unreadable.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return file.Sync()
}

// sealedWriter seals a stream of plaintext as it is written to it.
type sealedWriter struct {
	destination io.Writer
	stream      cipher.AEAD
	nonce       [sealNonceSize]byte
	buffer      []byte
	filled      int
	index       uint64
	closed      bool
}

// newSealedWriter writes the header and returns a writer that seals what it
// receives. The caller must Close it: the end mark is what tells a reader that
// the object is complete.
func newSealedWriter(key []byte, destination io.Writer) (*sealedWriter, error) {
	stream, err := newStream(key)
	if err != nil {
		return nil, err
	}
	writer := &sealedWriter{
		destination: destination,
		stream:      stream,
		buffer:      make([]byte, sealChunkSize),
	}
	if _, err := io.WriteString(destination, sealMagic); err != nil {
		return nil, fmt.Errorf("write sealed header: %w", err)
	}
	if _, err := rand.Read(writer.nonce[:]); err != nil {
		return nil, fmt.Errorf("draw nonce: %w", err)
	}
	if _, err := destination.Write(writer.nonce[:]); err != nil {
		return nil, fmt.Errorf("write sealed header: %w", err)
	}
	var chunk [4]byte
	binary.BigEndian.PutUint32(chunk[:], sealChunkSize)
	if _, err := destination.Write(chunk[:]); err != nil {
		return nil, fmt.Errorf("write sealed header: %w", err)
	}
	return writer, nil
}

func (w *sealedWriter) Write(plaintext []byte) (int, error) {
	if w.closed {
		return 0, errors.New("sealed writer is closed")
	}
	written := 0
	for len(plaintext) > 0 {
		room := sealChunkSize - w.filled
		take := len(plaintext)
		if take > room {
			take = room
		}
		copy(w.buffer[w.filled:], plaintext[:take])
		w.filled += take
		plaintext = plaintext[take:]
		written += take
		if w.filled == sealChunkSize {
			if err := w.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// Close writes the end mark, which is the only thing that distinguishes a
// complete object from one whose upload was interrupted.
func (w *sealedWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.flush(); err != nil {
		return err
	}
	var end [4]byte
	if _, err := w.destination.Write(end[:]); err != nil {
		return fmt.Errorf("write sealed end mark: %w", err)
	}
	return nil
}

func (w *sealedWriter) flush() error {
	if w.filled == 0 {
		return nil
	}
	associated := recordAssociatedData(w.nonce, w.index)
	sealed := w.stream.Seal(nil, w.nonce[:], w.buffer[:w.filled], associated)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(w.filled))
	if _, err := w.destination.Write(length[:]); err != nil {
		return fmt.Errorf("write sealed record: %w", err)
	}
	if _, err := w.destination.Write(sealed); err != nil {
		return fmt.Errorf("write sealed record: %w", err)
	}
	w.filled = 0
	w.index++
	return nil
}

// unseal copies a sealed stream to a destination, refusing anything that is not
// one. The writes happen as records are read, so an interrupted unseal leaves a
// partial *plaintext* file that the caller deletes — the object it came from is
// untouched, which is why the restore path can simply start over.
func unseal(key []byte, source io.Reader, destination io.Writer) error {
	stream, err := newStream(key)
	if err != nil {
		return err
	}
	reader := bufio.NewReaderSize(source, sealChunkSize)
	magic := make([]byte, len(sealMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return fmt.Errorf("%w: %v", ErrSealedStream, err)
	}
	if string(magic) != sealMagic {
		return fmt.Errorf("%w: magic %q", ErrSealedStream, magic)
	}
	var nonce [sealNonceSize]byte
	if _, err := io.ReadFull(reader, nonce[:]); err != nil {
		return fmt.Errorf("read sealed header: %w", err)
	}
	var chunk [4]byte
	if _, err := io.ReadFull(reader, chunk[:]); err != nil {
		return fmt.Errorf("read sealed header: %w", err)
	}
	if size := binary.BigEndian.Uint32(chunk[:]); size != sealChunkSize {
		// The chunk size is part of the format, not a per-object choice: a
		// reader that honoured the header would decrypt a stream written by a
		// writer whose choice it cannot check.
		return fmt.Errorf("%w: chunk size %d, want %d", ErrSealedStream, size, sealChunkSize)
	}

	ciphertext := make([]byte, sealChunkSize+stream.Overhead())
	// truncated is the one reason a reader stops early in every shape the cut
	// can take — between records, inside a length, inside a record — and it is
	// reported as one reason rather than three, because the operator's next
	// action is the same in all three.
	truncated := errors.New("sealed object ends before its end mark: the upload was interrupted, or the object is truncated")
	for index := uint64(0); ; index++ {
		if _, err := io.ReadFull(reader, chunk[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return truncated
			}
			return fmt.Errorf("read sealed record: %w", err)
		}
		length := binary.BigEndian.Uint32(chunk[:])
		if length == 0 {
			// The end mark is followed by nothing. Anything after it is not
			// something a writer of this format produced.
			if _, err := reader.Peek(1); err == nil {
				return errors.New("sealed object carries bytes after its end mark")
			}
			return nil
		}
		if length > sealChunkSize {
			return fmt.Errorf("sealed object declares a %d-byte record, larger than the format's %d", length, sealChunkSize)
		}
		sealed := ciphertext[:int(length)+stream.Overhead()]
		if _, err := io.ReadFull(reader, sealed); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return truncated
			}
			return fmt.Errorf("read sealed record: %w", err)
		}
		plaintext, err := stream.Open(nil, nonce[:], sealed, recordAssociatedData(nonce, index))
		if err != nil {
			return fmt.Errorf("sealed record %d does not authenticate: the object was edited, or the key is not the one it was sealed with", index)
		}
		if _, err := destination.Write(plaintext); err != nil {
			return fmt.Errorf("write unsealed record: %w", err)
		}
	}
}

// recordAssociatedData binds a record to its position in this object, so that a
// record moved, dropped or duplicated fails to authenticate.
func recordAssociatedData(base [sealNonceSize]byte, index uint64) []byte {
	associated := make([]byte, 0, sealNonceSize+8)
	associated = append(associated, base[:]...)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], index)
	return append(associated, counter[:]...)
}

func newStream(key []byte) (cipher.AEAD, error) {
	if len(key) != sealKeySize {
		return nil, fmt.Errorf("sealing key is %d bytes, want %d", len(key), sealKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("sealing cipher: %w", err)
	}
	stream, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sealing cipher: %w", err)
	}
	return stream, nil
}
