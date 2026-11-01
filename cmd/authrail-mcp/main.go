package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AuthRail/authrail-mcp/internal/config"
	"github.com/AuthRail/authrail-mcp/internal/mcpserver"
)

func main() {
	settings, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("Invalid MCP configuration", "error", err)
		os.Exit(1)
	}
	token := settings.ServiceToken
	if settings.ServiceTokenFile != "" {
		token, err = readCredential(settings.ServiceTokenFile)
		if err != nil {
			slog.Error("Cannot load private service credential")
			os.Exit(1)
		}
	}
	backend, err := mcpserver.NewBackend(mcpserver.Config{APIURL: settings.APIURL, ResourceURL: settings.ResourceURL, AuthorizationServerURL: settings.AuthorizationServerURL, ServiceToken: token, AllowLoopback: settings.AllowLoopback})
	if err != nil {
		slog.Error("Invalid MCP configuration", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: settings.Listen, Handler: mcpserver.Handler(backend), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("AuthRail MCP listening", "address", settings.Listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("MCP listener failed")
		os.Exit(1)
	}
}
func readCredential(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		return "", errors.New("credential file must be private")
	}
	data := make([]byte, 8193)
	n, err := file.Read(data)
	if err != nil {
		return "", err
	}
	token := strings.TrimSuffix(strings.TrimSuffix(string(data[:n]), "\n"), "\r")
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\x00 ") {
		return "", errors.New("invalid credential")
	}
	return token, nil
}
