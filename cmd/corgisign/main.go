// Command corgisign is the official command-line client for the CorgiSign
// public API (the /api/v1 surface). It is a thin, dependency-free wrapper over
// the Go SDK (github.com/Corgi-Star/corgisign-go) that authenticates with an
// organisation API key read from the environment.
//
// Configuration (environment):
//
//	CORGISIGN_API_KEY   required — the cs_live_… / cs_test_… secret
//	CORGISIGN_BASE_URL  optional — API origin (default https://v2.sign.corgiinsure.com)
//
// Usage:
//
//	corgisign whoami
//	corgisign templates list [--team <id>]
//	corgisign envelopes list [--status <s>] [--team <id>] [--limit <n>]
//	corgisign envelopes get <id>
//	corgisign envelopes create --template <id> --title <t> \
//	    --recipient role=signer,name=Jane,email=jane@x.com [--send]
//	corgisign envelopes create --document <file.pdf> --team <id> --title <t> \
//	    --recipient name=Jane,email=jane@x.com [--send]
//	corgisign envelopes send <id>
//	corgisign send <id>            # alias for "envelopes send"
//	corgisign version
//
// Add --json to most commands for the raw API JSON instead of a table.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// version is the CLI version, overridable at build time with
// -ldflags "-X main.version=…".
var version = "0.1.0"

// defaultBaseURL is the production CorgiSign origin used when CORGISIGN_BASE_URL
// is not set.
const defaultBaseURL = "https://v2.sign.corgiinsure.com"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	case "version", "--version", "-v":
		fmt.Printf("corgisign %s\n", version)
		return
	}

	ctx := context.Background()
	if err := dispatch(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

// dispatch routes the top-level command to its handler.
func dispatch(ctx context.Context, args []string) error {
	switch args[0] {
	case "whoami":
		return cmdWhoAmI(ctx, args[1:])
	case "templates":
		return cmdTemplates(ctx, args[1:])
	case "envelopes", "envelope", "env":
		return cmdEnvelopes(ctx, args[1:])
	case "send": // convenience alias for "envelopes send <id>"
		return cmdEnvelopeSend(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q (run `corgisign help`)", args[0])
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `corgisign — command-line client for the CorgiSign API

Environment:
  CORGISIGN_API_KEY    required — your organisation API key (cs_live_… / cs_test_…)
  CORGISIGN_BASE_URL   optional — API origin (default `+defaultBaseURL+`)

Commands:
  whoami                              Show the org/team/scopes your key resolves to
  templates list [--team <id>]        List reusable templates (with placeholders)
  envelopes list [--status <s>]       List envelopes (newest first)
                 [--team <id>] [--limit <n>]
  envelopes get <id>                  Show one envelope's status and recipients
  envelopes create ...                Create an envelope (see flags below)
  envelopes send <id>                 Send a draft envelope to its recipients
  send <id>                           Alias for "envelopes send"
  version                             Print the CLI version

envelopes create flags:
  --template <id>                     Create from a template (or use --document)
  --document <file.pdf>               Create from an inline PDF (requires --team)
  --team <id>                         Team that owns the envelope
  --title <t>                         Envelope title
  --recipient k=v,k=v   (repeatable)  role,placeholderId,name,email,signingOrder
  --field k=v,k=v       (repeatable)  role,type,value (prefill)
  --send                              Send immediately after creation

Global:
  --json                              Print the raw API JSON instead of a table

Examples:
  export CORGISIGN_API_KEY=cs_live_xxx
  corgisign whoami
  corgisign templates list
  corgisign envelopes create --template <tid> --title "NDA" \
    --recipient role=signer,name=Jane,email=jane@example.com --send
  corgisign envelopes list --status sent --limit 10
`)
}

// flag parsing helpers -------------------------------------------------------

// parsedFlags is a tiny flag bag: named string values, repeated values, and
// boolean switches, plus any leftover positional arguments.
type parsedFlags struct {
	str    map[string]string
	multi  map[string][]string
	bools  map[string]bool
	posita []string
}

// parseFlags consumes a simple "--name value", "--name=value", and "--bool"
// flag grammar. boolFlags names which flags take no value. multiFlags names
// which flags may repeat (their values accumulate).
func parseFlags(args []string, boolFlags, multiFlags map[string]bool) (*parsedFlags, error) {
	p := &parsedFlags{str: map[string]string{}, multi: map[string][]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			p.posita = append(p.posita, a)
			continue
		}
		name := strings.TrimPrefix(a, "--")
		var val string
		hasInline := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			val, name, hasInline = name[eq+1:], name[:eq], true
		}
		if boolFlags[name] {
			p.bools[name] = true
			continue
		}
		if !hasInline {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag --%s needs a value", name)
			}
			i++
			val = args[i]
		}
		if multiFlags[name] {
			p.multi[name] = append(p.multi[name], val)
		} else {
			p.str[name] = val
		}
	}
	return p, nil
}

// kvPairs parses "k=v,k=v" into a map. Values may themselves contain no commas.
func kvPairs(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}
