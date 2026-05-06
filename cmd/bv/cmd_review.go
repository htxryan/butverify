// `bv review <subcommand>` — manage reviews on review-enabled evidence
// sites (Evidence v2; pebble-xu6b).
//
// Subcommands:
//
//	bv review list [--site <id>] [--unacknowledged] [--format json|ids]
//	bv review get <review-id>
//	bv review acknowledge <review-id>
//	bv review request <site-id> --to <github-login>
//
// All subcommands authenticate with the persisted installation token
// (server scopes results to the calling tenant — EV2-U-7, EV2-U-12).
//
// `bv review list` applies a 2-second hard timeout when --format=ids is
// requested so the Stop hook (EV2-E-8) cannot stall a session-end on a
// slow API call. Other subcommands inherit the standard 60s client
// timeout.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/htxryan/butverify/internal/api"
)

func runReview(ctx context.Context, g globalContext, args []string) int {
	if len(args) == 0 {
		g.w.Error(toErrorEnvelope(usageError("review")))
		return 2
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "--help", "-h":
		printCommandHelp("review")
		return 0
	case "list":
		return runReviewList(ctx, g, rest)
	case "get":
		return runReviewGet(ctx, g, rest)
	case "acknowledge":
		return runReviewAcknowledge(ctx, g, rest)
	case "request":
		return runReviewRequest(ctx, g, rest)
	default:
		g.w.Error(toErrorEnvelope(fmt.Errorf("unknown review subcommand %q (expected list|get|acknowledge|request)", sub)))
		return 2
	}
}

const reviewListIDsTimeout = 2 * time.Second

// exitReviewerUnresolvable is the bv exit code for an EV2-N-7
// `reviewer_email_unknown` 422 — the request was processed but the
// reviewer has no resolvable email, so the agent must share manually.
// Distinct from the generic "1 = API error" so a script can branch on
// "request emitted, just notify out-of-band" vs "actually failed".
const exitReviewerUnresolvable = 1

// reviewSubFlagSet builds a stdlib FlagSet that prints `bv review`
// usage on -h. We don't route through cliref.NewFlagSet because that
// is keyed on top-level command names.
func reviewSubFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("review "+name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func runReviewList(ctx context.Context, g globalContext, args []string) int {
	fs := reviewSubFlagSet("list")
	siteID := fs.String("site", "", "filter by site_id")
	unacked := fs.Bool("unacknowledged", false, "only return reviews not yet acknowledged")
	format := fs.String("format", "json", "output format: json or ids")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "review", err)
	}
	if *format != "json" && *format != "ids" {
		g.w.Error(toErrorEnvelope(fmt.Errorf("--format must be json or ids (got %q)", *format)))
		return 2
	}

	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}

	// EV2-E-8: --format=ids is the hook path. Apply a 2s hard timeout so a
	// slow control-plane never blocks a session-end. JSON callers keep the
	// client's standard 60s timeout for sites with many reviews.
	if *format == "ids" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, reviewListIDsTimeout)
		defer cancel()
	}

	q := url.Values{}
	if *unacked {
		q.Set("unacknowledged", "true")
	}
	if *siteID != "" {
		q.Set("site_id", *siteID)
	}
	path := "/v1/reviews"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var resp api.ListReviewsResponse
	if err := client.Do(ctx, "GET", path, nil, &resp); err != nil {
		return reportError(g.w, err)
	}

	if *format == "ids" {
		// Stable line-based shape consumed by EV2-E-8 hooks: each line is
		// `<site_id>#<review_id>`, no header, no trailing whitespace, and
		// nothing on stdout when there are zero results so the hook can
		// branch on `[ -s output ]`.
		for _, r := range resp.Reviews {
			fmt.Printf("%s#%s\n", strings.TrimSpace(r.SiteID), strings.TrimSpace(r.ReviewID))
		}
		return 0
	}

	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	// Default human format: emit JSON document on stdout for agent
	// ergonomics. The spec (§8) defines `bv review list` as a
	// JSON-returning command; humans still get readable indented output
	// without needing --json. Stream straight to os.Stdout, matching
	// the `bv manifest` pattern (callers pipe into `jq`).
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
	return 0
}

func runReviewGet(ctx context.Context, g globalContext, args []string) int {
	fs := reviewSubFlagSet("get")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "review", err)
	}
	pos := fs.Args()
	if len(pos) != 1 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv review get <review-id>")))
		return 2
	}
	reviewID := pos[0]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.GetReviewResponse
	if err := client.Do(ctx, "GET", "/v1/reviews/"+url.PathEscape(reviewID), nil, &resp); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
	return 0
}

func runReviewAcknowledge(ctx context.Context, g globalContext, args []string) int {
	fs := reviewSubFlagSet("acknowledge")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "review", err)
	}
	pos := fs.Args()
	if len(pos) != 1 {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv review acknowledge <review-id>")))
		return 2
	}
	reviewID := pos[0]
	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	var resp api.AcknowledgeReviewResponse
	if err := client.Do(ctx, "PATCH", "/v1/reviews/"+url.PathEscape(reviewID)+"/acknowledge", nil, &resp); err != nil {
		return reportError(g.w, err)
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(resp)
		return 0
	}
	if resp.AlreadyAcknowledged {
		g.w.Human("Review %s was already acknowledged at %s", resp.ReviewID, resp.AcknowledgedAt)
	} else {
		g.w.Human("Acknowledged %s at %s", resp.ReviewID, resp.AcknowledgedAt)
	}
	return 0
}

func runReviewRequest(ctx context.Context, g globalContext, args []string) int {
	// Hand-roll arg parsing so the spec's `bv review request <site-id> --to <login>`
	// shape works (Go's `flag` package stops at the first non-flag arg, so a
	// FlagSet would treat `--to` as a positional once `<site-id>` precedes it).
	var siteID, to string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			printCommandHelp("review")
			return 0
		case a == "--to":
			if i+1 >= len(args) {
				g.w.Error(toErrorEnvelope(errors.New("--to requires a value")))
				return 2
			}
			i++
			to = args[i]
		case strings.HasPrefix(a, "--to="):
			to = strings.TrimPrefix(a, "--to=")
		case strings.HasPrefix(a, "-"):
			g.w.Error(toErrorEnvelope(fmt.Errorf("unknown flag %q", a)))
			return 2
		default:
			if siteID != "" {
				g.w.Error(toErrorEnvelope(errors.New("usage: bv review request <site-id> --to <github-login>")))
				return 2
			}
			siteID = a
		}
	}
	if siteID == "" {
		g.w.Error(toErrorEnvelope(errors.New("usage: bv review request <site-id> --to <github-login>")))
		return 2
	}
	if to == "" {
		g.w.Error(toErrorEnvelope(errors.New("--to <github-login> is required")))
		return 2
	}

	client, _, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	body := api.RequestReviewBody{ReviewerLogin: to}
	var success api.RequestReviewSuccess
	err = client.Do(ctx, "POST", "/v1/sites/"+url.PathEscape(siteID)+"/review-requests", body, &success)
	if err == nil {
		if g.w.IsJSON() {
			_ = g.w.JSON(success)
			return 0
		}
		g.w.Human("Requested review from @%s — notification %s", success.ReviewerLogin, success.Notification)
		return 0
	}

	// EV2-N-7: 422 carries a non-envelope shape (`{requested:false, error,
	// site_url}`) the agent must surface verbatim so it can share the URL
	// manually. parseError() falls through to UNKNOWN code on this body
	// because there's no `error.code` field at the top level — recover
	// the structured shape from APIError.Raw.
	var ae *api.APIError
	if errors.As(err, &ae) && ae.Status == 422 && len(ae.Raw) > 0 {
		var unresolvable api.RequestReviewUnresolvable
		if jsonErr := json.Unmarshal(ae.Raw, &unresolvable); jsonErr == nil && unresolvable.Error == "reviewer_email_unknown" {
			if g.w.IsJSON() {
				_ = g.w.JSON(unresolvable)
				return exitReviewerUnresolvable
			}
			g.w.Human("Could not resolve an email for @%s; share %s manually.", to, unresolvable.SiteURL)
			return exitReviewerUnresolvable
		}
	}
	return reportError(g.w, err)
}

