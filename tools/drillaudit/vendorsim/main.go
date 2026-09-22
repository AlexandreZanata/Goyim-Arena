// Command vendorsim is the provider stand-in of the disaster drill (P20-T05).
//
// The email exercise has two directions: the provider unreachable, and the
// provider answering once it is back. The first is a closed port and needs
// nothing. The second needs a provider, and the deployment deliberately refuses
// to point the adapter anywhere but the real API — the endpoint is not
// configurable precisely so that a credential can only ever be sent to the
// provider. A drill that read "the provider answered" without a provider would be
// asserting its own hope, so this command *is* the provider for the length of the
// exercise:
//
//   - it issues a certificate for the provider's own hostname and the exercise
//     hands it to the worker as its trust root (`SSL_CERT_FILE`), so the
//     adapter's TLS verification still happens and still means something;
//   - it speaks the proxy protocol the worker's transport uses for HTTPS
//     (`HTTPS_PROXY`), so nothing in the adapter is bypassed: the request the
//     stand-in answers is the request the real worker sent;
//   - it answers the send endpoint with the receipt shape the provider uses, and
//     appends a line per answered delivery, so "delivered" is a fact of a file.
//
// It answers nothing else — an unknown route is a 404 — and it records only the
// shape of what arrived, never a rendered message: the rule about real data
// applies to a drill's own artifacts too.
package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// providerHost is the hostname the certificate is issued for: the host the
// adapter's endpoint names, so the verification the worker performs is the real
// one against a certificate it was given out of band.
const providerHost = "api.resend.com"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: vendorsim -dir <directory> [-addr 127.0.0.1:0] [-log <file>]")
	}
	dir := flag.String("dir", "", "directory to write the certificate and the key into")
	addr := flag.String("addr", "127.0.0.1:0", "address to listen on; port 0 asks the kernel for one")
	logPath := flag.String("log", "", "where to append one line per answered delivery")
	flag.Parse()

	if *dir == "" {
		log.SetFlags(0)
		log.Fatal("vendorsim: -dir is required")
	}

	certificate, certPEM, keyPEM, err := issueCertificate()
	if err != nil {
		log.SetFlags(0)
		log.Fatalf("vendorsim: %v", err)
	}
	certPath := filepath.Join(*dir, "vendorsim.crt")
	keyPath := filepath.Join(*dir, "vendorsim.key")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		log.SetFlags(0)
		log.Fatalf("vendorsim: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		log.SetFlags(0)
		log.Fatalf("vendorsim: %v", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.SetFlags(0)
		log.Fatalf("vendorsim: %v", err)
	}
	defer listener.Close()

	// The port and the trust root are what the exercise needs from the process,
	// and they are printed on one line so the script never guesses a port.
	fmt.Printf("vendorsim port=%d cert=%s\n", listener.Addr().(*net.TCPAddr).Port, certPath)

	config := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	stand := &standIn{logPath: *logPath}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// One connection per delivery, each on its own goroutine, so the retry
		// of the exercise works exactly like the first attempt.
		go serve(conn, config, stand)
	}
}

// serve handles one accepted connection: it reads the CONNECT the worker's
// transport sends, accepts it, and only then starts TLS. The TLS stream continues
// from the *same* reader the request was read from, because bytes the reader
// buffered past the CONNECT headers belong to the handshake — a transport that
// sent them early would otherwise be talking to a server that lost its beginning.
func serve(conn net.Conn, config *tls.Config, stand *standIn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if request.Method != http.MethodConnect {
		_, _ = io.WriteString(conn, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")

	presented := tls.Server(&bufferedConn{Conn: conn, reader: reader}, config)
	if err := presented.Handshake(); err != nil {
		fmt.Fprintf(os.Stderr, "vendorsim: handshake: %v\n", err)
		return
	}
	stand.answer(presented)
}

// bufferedConn is a connection whose reads continue from a buffered reader:
// everything the CONNECT request did not consume is still there for TLS.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(buffer []byte) (int, error) { return c.reader.Read(buffer) }

// standIn answers the send endpoint of the provider. One request per connection
// is deliberate: the answer is written whole and the connection closes, so a
// transport reads a complete response and never waits on a keep-alive the
// exercise did not ask for.
type standIn struct {
	logPath string
	count   atomic.Int64
}

func (s *standIn) answer(conn net.Conn) {
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		fmt.Fprintf(os.Stderr, "vendorsim: request: %v\n", err)
		return
	}
	status := http.StatusNotFound
	body := []byte(`{"error":"vendorsim: not the send endpoint"}`)
	if request.Method == http.MethodPost && request.URL.Path == "/emails" {
		payload, _ := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		receipt := fmt.Sprintf("vendorsim-%d", s.count.Add(1))
		line := fmt.Sprintf("%s %s receipt=%s bytes=%d idempotency=%t\n",
			time.Now().UTC().Format(time.RFC3339), request.URL.Path, receipt, len(payload),
			request.Header.Get("Idempotency-Key") != "")
		if s.logPath != "" {
			if file, err := os.OpenFile(s.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = file.WriteString(line)
				file.Close()
			}
		}
		fmt.Fprint(os.Stdout, line)
		status = http.StatusOK
		body, _ = json.Marshal(map[string]string{"id": receipt})
	}

	response := &http.Response{
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}, "Connection": []string{"close"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	_ = response.Write(conn)
}

// issueCertificate builds the provider's key pair and a certificate for the
// provider's hostname: self-signed, valid for the exercise, and used as its own
// trust root by the worker.
func issueCertificate() (tls.Certificate, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: providerHost, Organization: []string{"disaster drill provider stand-in"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{providerHost},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	return pair, certPEM, keyPEM, nil
}
