package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	corgisign "github.com/Corgi-Star/corgisign-go-sdk"
)

// newClient builds an SDK client from the environment. CORGISIGN_API_KEY is
// required; the base URL falls back to the production origin.
func newClient() (*corgisign.Client, error) {
	key := strings.TrimSpace(os.Getenv("CORGISIGN_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("CORGISIGN_API_KEY is not set (export your cs_live_… key)")
	}
	base := strings.TrimSpace(os.Getenv("CORGISIGN_BASE_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("CORGISIGN_API_URL")) // accepted alias
	}
	if base == "" {
		base = defaultBaseURL
	}
	return corgisign.New(corgisign.Options{
		APIKey:    key,
		BaseURL:   base,
		UserAgent: "corgisign-cli/" + version,
	}), nil
}

// printJSON writes v as indented JSON to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// dash renders an empty string as a dash for table output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
