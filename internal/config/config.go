// Package config loads the MCP service settings.
package config

import (
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
)

// Config holds the service URLs, listener address and credential source.
type Config struct {
	Listen                 string
	APIURL                 string
	ResourceURL            string
	AuthorizationServerURL string
	ServiceToken           string
	ServiceTokenFile       string
	AllowLoopback          bool
}

// Load reads defaults, environment values, then explicit command-line flags.
// We use the process environment and leave .env files to deployment tooling.
func Load(args []string) (Config, error) {
	k := koanf.New(".")
	if err := k.Load(confmap.Provider(map[string]any{"listen": "127.0.0.1:3016", "allow_loopback": "false"}, "."), nil); err != nil {
		return Config{}, err
	}
	if err := k.Load(env.Provider(".", env.Opt{Prefix: "AUTHRAIL_MCP_", TransformFunc: func(key, value string) (string, any) {
		return strings.ToLower(strings.TrimPrefix(key, "AUTHRAIL_MCP_")), value
	}}), nil); err != nil {
		return Config{}, err
	}
	flags := flag.NewFlagSet("authrail-mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", k.String("listen"), "HTTP listener")
	api := flags.String("api", k.String("api_url"), "AuthRail API origin")
	resource := flags.String("resource", k.String("resource_url"), "Public MCP resource URL")
	issuer := flags.String("authorization-server", k.String("authorization_server_url"), "Public OAuth issuer URL")
	credential := flags.String("service-token-file", k.String("service_token_file"), "Owner-private service credential file")
	local := flags.String("allow-loopback", k.String("allow_loopback"), "Permit HTTP loopback development URLs")
	legacyLocal := flags.Bool("local", false, "Permit HTTP loopback development URLs")
	if err := flags.Parse(args); err != nil {
		return Config{}, errors.New("invalid MCP command-line options")
	}
	if flags.NArg() != 0 {
		return Config{}, errors.New("unexpected MCP command-line arguments")
	}
	if *local != "true" && *local != "false" {
		return Config{}, errors.New("AUTHRAIL_MCP_ALLOW_LOOPBACK must be true or false")
	}
	c := Config{Listen: *listen, APIURL: *api, ResourceURL: *resource, AuthorizationServerURL: *issuer, ServiceToken: k.String("service_token"), ServiceTokenFile: *credential, AllowLoopback: *local == "true" || *legacyLocal}
	if c.APIURL == "" || c.ResourceURL == "" {
		return Config{}, errors.New("AUTHRAIL_MCP_API_URL and AUTHRAIL_MCP_RESOURCE_URL are required")
	}
	if c.ServiceToken != "" && c.ServiceTokenFile != "" {
		return Config{}, errors.New("configure either AUTHRAIL_MCP_SERVICE_TOKEN or AUTHRAIL_MCP_SERVICE_TOKEN_FILE, not both")
	}
	if c.ServiceToken == "" && c.ServiceTokenFile == "" {
		return Config{}, errors.New("a private MCP service credential is required")
	}
	return c, nil
}
