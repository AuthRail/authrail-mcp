package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AuthRail/authrail-mcp/protectedmcp"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "AuthRail protected MCP:", err)
		os.Exit(1)
	}
}
func run() error {
	k := koanf.New(".")
	if err := k.Load(env.Provider(".", env.Opt{Prefix: "AUTHRAIL_PROTECTED_MCP_", TransformFunc: func(key, value string) (string, any) { return key[len("AUTHRAIL_PROTECTED_MCP_"):], value }}), nil); err != nil {
		return errors.New("connector environment is invalid")
	}
	flags := flag.NewFlagSet("authrail-protected-mcp", flag.ContinueOnError)
	configPath := flags.String("config", k.String("CONFIG"), "private connector configuration file")
	bind := flags.Bool("bind", false, "bind chosen tools to exact reviewed actions selected in the local sidecar")
	outputPath := flags.String("output", "", "new private configuration path for --bind; never overwrites a file")
	idempotencyField := flags.String("idempotency-field", "", "connector-owned argument to exclude from inspected Operation inputs")
	toolName := flags.String("tool", "", "inspect only this exact downstream tool name")
	inspect := flags.Bool("inspect", false, "list downstream tool contracts for review; never execute a tool")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid command options")
	}
	if *configPath == "" || flags.NArg() != 0 || *bind && (*inspect || *outputPath == "") || !*bind && *outputPath != "" || !*inspect && (*idempotencyField != "" || *toolName != "") {
		return errors.New("supply --config with the private operator configuration")
	}
	settings, err := protectedmcp.LoadSettings(*configPath)
	if err != nil {
		return err
	}
	if value := k.String("SIDECAR_URL"); value != "" {
		settings.SidecarURL = value
	}
	if value := k.String("DOWNSTREAM_URL"); value != "" {
		settings.DownstreamURL = value
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	downstream, err := protectedmcp.ConnectDownstream(ctx, settings)
	if err != nil {
		return errors.New("downstream connection failed; check its private authentication and HTTPS address")
	}
	defer downstream.Close()
	if *inspect {
		tools, err := protectedmcp.InspectTools(connectCtx, downstream)
		if err != nil {
			return errors.New("downstream tool discovery failed")
		}
		found := false
		for _, tool := range tools {
			if *toolName != "" && tool.Name != *toolName {
				continue
			}
			found = true
			digest, err := protectedmcp.SchemaDigest(tool)
			if err != nil {
				return err
			}
			input, err := protectedmcp.ReviewedInputSchema(tool.InputSchema, *idempotencyField)
			if err != nil {
				return err
			}
			if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"name": tool.Name, "schemaSha256": digest, "downstreamInputSchema": tool.InputSchema, "operationInputSchema": input, "downstreamOutputSchema": tool.OutputSchema, "operationResultSchema": protectedmcp.OperationResultSchema(tool), "review": "Draft input only. Review the real effect, idempotency, identity and recovery contract before binding an Operation."}); err != nil {
				return err
			}
		}
		if *toolName != "" && !found {
			return errors.New("the selected downstream tool was not found")
		}
		return nil
	}
	if *bind {
		prepared, err := protectedmcp.PrepareBindings(connectCtx, settings, downstream)
		if err != nil {
			return err
		}
		if err = protectedmcp.SavePreparedSettings(*outputPath, prepared); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "bound", "configurationFile": *outputPath, "next": "Connect your agent to this command with --config pointing at the saved configuration. Remove equivalent direct tool access and keep credentials outside the agent before claiming enforced protection. No business action has run."})
	}
	connector, err := protectedmcp.Open(connectCtx, settings, downstream)
	if err != nil {
		return err
	}
	defer connector.Close()
	if err = connector.Server().Run(ctx, &mcp.StdioTransport{}); err != nil {
		return errors.New("protected MCP session stopped")
	}
	return nil
}
