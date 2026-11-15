package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AuthRail/authrail-mcp/internal/gateway"
	"github.com/AuthRail/authrail-mcp/internal/gateway/wire"
	"github.com/AuthRail/authrail-mcp/internal/healthprobe"
	"github.com/AuthRail/authrail-mcp/internal/telemetry"
	"google.golang.org/grpc"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "health" {
		address := "http://127.0.0.1:3019/health/ready"
		if len(os.Args) > 2 {
			address = os.Args[2]
		}
		if healthprobe.Run(address) != nil {
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry := telemetry.Start(ctx, "authrail-gateway-adapter")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdownTelemetry(ctx)
	}()
	privateHTTP := os.Getenv("AUTHRAIL_GATEWAY_ALLOW_PRIVATE_HTTP") == "true"
	api, err := gateway.NewAPI(os.Getenv("AUTHRAIL_GATEWAY_API_URL"), os.Getenv("MCP_SERVICE_TOKEN"), privateHTTP)
	if err != nil {
		slog.Error("Gateway private API configuration is invalid")
		os.Exit(1)
	}
	configuration, err := gateway.NewConfiguration(api, setting("AUTHRAIL_GATEWAY_CONFIG", "/var/lib/authrail-gateway/config.json"), setting("AUTHRAIL_GATEWAY_HOOK_HOST", "gateway-adapter:3020"))
	if err != nil {
		slog.Error("Gateway private configuration storage is unavailable")
		os.Exit(1)
	}
	dispatcher, err := gateway.NewDispatcher(api, configuration, setting("AUTHRAIL_GATEWAY_URL", "http://agentgateway:3018"), privateHTTP)
	if err != nil {
		slog.Error("Gateway transport configuration is invalid")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", setting("AUTHRAIL_GATEWAY_GRPC_LISTEN", ":3020"))
	if err != nil {
		slog.Error("Gateway hook listener is unavailable")
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(512<<10), grpc.MaxSendMsgSize(512<<10))
	wire.RegisterExtMcpServer(grpcServer, &gateway.Hooks{API: api})
	httpServer := &http.Server{Addr: setting("AUTHRAIL_GATEWAY_LISTEN", ":3019"), Handler: telemetry.HTTP("gateway.dispatch", dispatcher.Handler()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("Gateway hook listener stopped")
			stop()
		}
	}()
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Gateway dispatcher stopped")
			stop()
		}
	}()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			check, cancel := context.WithTimeout(ctx, 8*time.Second)
			err := configuration.Sync(check)
			cancel()
			if err != nil {
				slog.Warn("Gateway route configuration is unavailable")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	slog.Info("AuthRail gateway adapter listening")
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	done := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-shutdown.Done():
		grpcServer.Stop()
	}
}

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
