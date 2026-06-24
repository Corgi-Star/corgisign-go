package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	corgisign "github.com/Corgi-Star/corgisign-go"
)

// cmdWhoAmI implements `corgisign whoami`.
func cmdWhoAmI(ctx context.Context, args []string) error {
	p, err := parseFlags(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	id, err := c.WhoAmI(ctx)
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(id)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "Key\t%s (%s)\n", dash(id.Name), dash(id.Prefix))
	fmt.Fprintf(tw, "Organisation\t%s\t%s\n", dash(id.OrganisationName), id.OrganisationID)
	if id.TeamID != nil {
		fmt.Fprintf(tw, "Team (pinned)\t%s\n", *id.TeamID)
	} else {
		fmt.Fprintf(tw, "Team\torg-wide\n")
	}
	fmt.Fprintf(tw, "Environment\t%s\n", dash(id.Environment))
	fmt.Fprintf(tw, "Scopes\t%s\n", strings.Join(id.Scopes, ", "))
	return tw.Flush()
}

// cmdTemplates implements `corgisign templates …`.
func cmdTemplates(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return fmt.Errorf("usage: corgisign templates list [--team <id>] [--json]")
	}
	p, err := parseFlags(args[1:], map[string]bool{"json": true}, nil)
	if err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	var opts []corgisign.RequestOption
	if t := p.str["team"]; t != "" {
		opts = append(opts, corgisign.WithTeamID(t))
	}
	tmpls, err := c.Templates.List(ctx, opts...)
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(tmpls)
	}
	if len(tmpls) == 0 {
		fmt.Println("no templates")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tROLES")
	for _, t := range tmpls {
		roles := make([]string, 0, len(t.Recipients))
		for _, r := range t.Recipients {
			roles = append(roles, dash(r.Role))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.ID, dash(t.Title), strings.Join(roles, ", "))
	}
	return tw.Flush()
}

// cmdEnvelopes routes the `corgisign envelopes …` subcommands.
func cmdEnvelopes(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: corgisign envelopes <list|get|create|send> …")
	}
	switch args[0] {
	case "list", "ls":
		return cmdEnvelopeList(ctx, args[1:])
	case "get", "show":
		return cmdEnvelopeGet(ctx, args[1:])
	case "create", "new":
		return cmdEnvelopeCreate(ctx, args[1:])
	case "send":
		return cmdEnvelopeSend(ctx, args[1:])
	default:
		return fmt.Errorf("unknown envelopes subcommand %q", args[0])
	}
}

func cmdEnvelopeList(ctx context.Context, args []string) error {
	p, err := parseFlags(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	params := corgisign.ListEnvelopesParams{
		Status: corgisign.EnvelopeStatus(p.str["status"]),
		TeamID: p.str["team"],
	}
	if l := p.str["limit"]; l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return fmt.Errorf("--limit must be a number: %v", err)
		}
		params.Limit = n
	}
	envs, err := c.Envelopes.List(ctx, params)
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(envs)
	}
	if len(envs) == 0 {
		fmt.Println("no envelopes")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tTITLE\tCREATED")
	for _, e := range envs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.ID, e.Status, dash(e.Title), e.CreatedAt.Format("2006-01-02 15:04"))
	}
	return tw.Flush()
}

func cmdEnvelopeGet(ctx context.Context, args []string) error {
	p, err := parseFlags(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return err
	}
	if len(p.posita) == 0 {
		return fmt.Errorf("usage: corgisign envelopes get <id> [--json]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	env, err := c.Envelopes.Get(ctx, p.posita[0])
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(env)
	}
	printEnvelope(env)
	return nil
}

func cmdEnvelopeCreate(ctx context.Context, args []string) error {
	p, err := parseFlags(args,
		map[string]bool{"json": true, "send": true},
		map[string]bool{"recipient": true, "field": true})
	if err != nil {
		return err
	}
	c, err := newClient()
	if err != nil {
		return err
	}

	req := corgisign.CreateEnvelope{
		TemplateID: p.str["template"],
		TeamID:     p.str["team"],
		Title:      p.str["title"],
		Send:       p.bools["send"],
	}
	for _, rs := range p.multi["recipient"] {
		kv := kvPairs(rs)
		r := corgisign.Recipient{
			Role:          kv["role"],
			PlaceholderID: kv["placeholderId"],
			Name:          kv["name"],
			Email:         kv["email"],
		}
		if so := kv["signingOrder"]; so != "" {
			r.SigningOrder, _ = strconv.Atoi(so)
		}
		req.Recipients = append(req.Recipients, r)
	}
	for _, fs := range p.multi["field"] {
		kv := kvPairs(fs)
		req.Fields = append(req.Fields, corgisign.FieldPrefill{
			Role: kv["role"], Type: kv["type"], Value: kv["value"],
		})
	}
	if doc := p.str["document"]; doc != "" {
		pdf, err := os.ReadFile(doc)
		if err != nil {
			return fmt.Errorf("reading --document: %w", err)
		}
		name := doc
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		req.Document = corgisign.DocumentFromBytes(name, pdf)
	}
	if req.TemplateID == "" && req.Document == nil {
		return fmt.Errorf("provide either --template <id> or --document <file.pdf>")
	}

	env, err := c.Envelopes.Create(ctx, req)
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(env)
	}
	fmt.Printf("created envelope %s (%s)\n", env.ID, env.Status)
	printEnvelope(env)
	return nil
}

// cmdEnvelopeSend implements `corgisign envelopes send <id>` and `corgisign send <id>`.
func cmdEnvelopeSend(ctx context.Context, args []string) error {
	p, err := parseFlags(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return err
	}
	if len(p.posita) == 0 {
		return fmt.Errorf("usage: corgisign envelopes send <id>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	env, err := c.Envelopes.Send(ctx, p.posita[0])
	if err != nil {
		return err
	}
	if p.bools["json"] {
		return printJSON(env)
	}
	fmt.Printf("sent envelope %s (%s)\n", env.ID, env.Status)
	return nil
}

// printEnvelope renders an envelope's header and recipient table.
func printEnvelope(env *corgisign.Envelope) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "Envelope\t%s\n", env.ID)
	fmt.Fprintf(tw, "Title\t%s\n", dash(env.Title))
	fmt.Fprintf(tw, "Status\t%s\n", env.Status)
	_ = tw.Flush()
	if len(env.Recipients) == 0 {
		return
	}
	fmt.Println("Recipients:")
	rw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(rw, "  ROLE\tNAME\tEMAIL\tSTATUS")
	for _, r := range env.Recipients {
		fmt.Fprintf(rw, "  %s\t%s\t%s\t%s\n", dash(r.Role), dash(r.Name), dash(r.Email), dash(r.Status))
	}
	_ = rw.Flush()
}
