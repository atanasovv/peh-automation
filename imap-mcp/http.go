package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tokens and public hostnames are deployment settings, never tool arguments.
func mailHTTPHandler(server *mcp.Server, token, publicHost string) (http.Handler, error) {
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("MCP_AUTH_TOKEN must contain at least 32 characters without whitespace")
	}
	publicURL, err := url.Parse("https://" + publicHost)
	if err != nil || publicHost == "" || publicURL.Host != publicHost || publicURL.Path != "" ||
		publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return nil, errors.New("MCP_PUBLIC_HOST must be a hostname, optionally with a port")
	}
	expected := sha256.Sum256([]byte("Bearer " + token))
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
		// The outer handler enforces the configured public Host and Origin even
		// when a reverse proxy or health test connects through loopback.
		DisableLocalhostProtection: true,
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "ok")
	})
	authenticatedMCP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !strings.EqualFold(r.Host, publicHost) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "https://"+publicHost {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		supplied := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="superhosting-mail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Draft text is limited separately. This also bounds JSON requests before decoding.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		transport.ServeHTTP(w, r)
	})
	mux.Handle("/{$}", authenticatedMCP)
	mux.Handle("/mcp", authenticatedMCP)
	return mux, nil
}

func runHTTP(server *mcp.Server) error {
	handler, err := mailHTTPHandler(server, os.Getenv("MCP_AUTH_TOKEN"), os.Getenv("MCP_PUBLIC_HOST"))
	if err != nil {
		return err
	}
	address := os.Getenv("MCP_LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}
	listener := &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 6 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = listener.Shutdown(shutdownCtx)
		case <-finished:
		}
	}()
	defer close(finished)
	err = listener.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
