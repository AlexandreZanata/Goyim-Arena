// Package httpserver owns the transport lifecycle of Goyim Arena (P02-T05):
// a hardened net/http server with explicit timeouts, a bounded header size,
// graceful shutdown driven by a context, and the health endpoints required
// by the plan.
//
// Hardening policy (docs/DEPLOYMENT.md, SCALABILITY.md):
//   - ReadHeaderTimeout guards against slowloris-style header starvation;
//   - ReadTimeout, WriteTimeout and IdleTimeout bound every request phase;
//   - MaxHeaderBytes is tightened well below the permissive stdlib default;
//   - shutdown always releases the listener first and then waits, within a
//     fixed deadline, for in-flight requests to finish.
//
// The package stays transport-only: routing composition of the platform
// middleware (request ID correlation) and signal handling live in cmd/.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/requestid"
	"github.com/AlexandreZanata/Goyim-Arena/internal/ports"
)

// Default hardening values. They are constants, not configuration, because
// the plan does not expose them as tunables yet; operators get them by
// default on every environment.
const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultReadTimeout       = 20 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 120 * time.Second

	// defaultMaxHeaderBytes tightens the stdlib default (1 MiB) to 64 KiB:
	// generous for session cookies and correlation headers, hostile to
	// header-abuse memory pressure.
	defaultMaxHeaderBytes = 64 << 10

	// shutdownTimeout bounds the graceful drain of in-flight requests after
	// the shutdown signal. Processes that exceed it are deliberately allowed
	// to be killed by the supervisor.
	shutdownTimeout = 10 * time.Second
)

// Options configures a Server. The timeout and header fields default to the
// hardened constants above when left zero; tests may tighten them further.
type Options struct {
	// Addr is the TCP listen address, for example 127.0.0.1:8080.
	Addr string

	// Handler is the root handler; composition (request ID, health routes)
	// is built with NewMux in cmd/.
	Handler http.Handler

	// Logger receives lifecycle records (listening, shutdown). Required.
	Logger *slog.Logger

	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// Server wraps a hardened *http.Server with an explicit listen/shutdown
// lifecycle driven by a context.
type Server struct {
	// HTTP exposes the configured server for inspection by tests.
	HTTP *http.Server

	logger   *slog.Logger
	listener net.Listener
}

// New builds a hardened server, applying the package defaults to any unset
// timeout or header limit.
func New(options Options) (*Server, error) {
	if options.Addr == "" {
		return nil, errors.New("httpserver: listen address is required")
	}
	if options.Handler == nil {
		return nil, errors.New("httpserver: handler is required")
	}
	if options.Logger == nil {
		return nil, errors.New("httpserver: logger is required")
	}

	readHeaderTimeout := options.ReadHeaderTimeout
	if readHeaderTimeout == 0 {
		readHeaderTimeout = defaultReadHeaderTimeout
	}
	readTimeout := options.ReadTimeout
	if readTimeout == 0 {
		readTimeout = defaultReadTimeout
	}
	writeTimeout := options.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = defaultWriteTimeout
	}
	idleTimeout := options.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = defaultIdleTimeout
	}
	maxHeaderBytes := options.MaxHeaderBytes
	if maxHeaderBytes == 0 {
		maxHeaderBytes = defaultMaxHeaderBytes
	}

	return &Server{
		HTTP: &http.Server{
			Addr:              options.Addr,
			Handler:           options.Handler,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			MaxHeaderBytes:    maxHeaderBytes,
		},
		logger: options.Logger,
	}, nil
}

// Listen binds the TCP listener eagerly, so a bad address or a busy port
// fails fast before the serve loop starts.
func (server *Server) Listen() error {
	listener, err := net.Listen("tcp", server.HTTP.Addr)
	if err != nil {
		return err
	}
	server.listener = listener
	return nil
}

// Addr returns the real bound address after Listen (resolving port 0);
// before Listen it returns the configured address.
func (server *Server) Addr() string {
	if server.listener != nil {
		return server.listener.Addr().String()
	}
	return server.HTTP.Addr
}

// Run serves until ctx is cancelled (for example by SIGTERM at the process
// edge), then performs a graceful shutdown: the listener is released
// immediately and in-flight requests get shutdownTimeout to finish.
//
// A context-driven stop returns nil; http.ErrServerClosed is never treated
// as an error. Real listener failures are returned.
func (server *Server) Run(ctx context.Context) error {
	if server.listener == nil {
		return errors.New("httpserver: Run called before Listen")
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.HTTP.Serve(server.listener)
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		server.logger.Info("http server: shutting down gracefully")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErr := server.HTTP.Shutdown(shutdownCtx)
		<-serveResult // Serve always returns ErrServerClosed after Shutdown.

		if shutdownErr != nil {
			server.logger.Error("http server: graceful shutdown failed", slog.String("error", shutdownErr.Error()))
			return shutdownErr
		}
		server.logger.Info("http server: graceful shutdown complete")
		return nil
	}
}

// statusBody is the single-field JSON document of the health endpoints.
type statusBody struct {
	Status string `json:"status"`
}

// LiveHandler reports liveness: the process is running. It never inspects
// dependencies, so a wedged database cannot make the supervisor restart a
// healthy-but-busy process (readiness is the deployment gate instead).
func LiveHandler() http.Handler {
	return writeStatus("live")
}

// ReadyHandler reports readiness. At this stage it validates process and
// configuration only — both are proven before the serve loop starts — and
// always answers ready. Dependency checks arrive with the database phase
// and must extend this handler, not the liveness one.
func ReadyHandler() http.Handler {
	return writeStatus("ready")
}

// writeStatus renders the fixed JSON health document.
func writeStatus(status string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(statusBody{Status: status})
	})
}

// NewMux composes the platform router from the route registry (routes.go):
// request ID correlation around the health routes registered with explicit
// method patterns, so wrong methods answer 405 automatically. Registration
// failures (duplicate or malformed registry entries) return an error
// instead of panicking at boot.
func NewMux(ids ports.IDGenerator) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := RegisterAll(mux, RegisteredRoutes()); err != nil {
		return nil, err
	}
	return requestid.Middleware(ids, mux), nil
}
