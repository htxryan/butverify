// Shared push pipeline used by `bv push`, `bv report --push`, and
// `bv dashboard --push`. The pipeline is the same in all three cases:
//
//   1. Generate (or accept) an upload_id.
//   2. POST /v1/sites — get a signed PUT URL + max-bytes cap. The server may
//      reject this with 402 if the tenant has hit the templated-site
//      fairness counter (O-3) when `template` is non-empty.
//   3. Bundle the directory into a tar.
//   4. PUT the tar to staging.
//   5. POST /v1/sites/{id}/finalize.
//   6. Emit the result (JSON or human).
//
// The original `runPush` body is preserved verbatim except for:
//   - upload_id generation moved out (the templated commands generate one
//     before this helper is called so they can include it in error output)
//   - `template` argument threads through CreateSiteRequest.Template
//
// We intentionally keep this in package main rather than a sibling internal
// package — the helper depends on the package-level newClient + reportError
// utilities, and pulling those into another package would expand the
// surface area visible to tests in cmd/bv/.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/internal/config"
	"github.com/htxryan/butverify/pkg/tarbundle"
)

// pushOptions bundles the parameters of a single push pipeline run.
type pushOptions struct {
	dir               string
	sourcePath        string
	uploadID          string
	ttlSeconds        int64 // 0 = use server default
	template          string
	includeHidden     bool
	skipGitleaksCheck bool
	imageQuality      int
	modeOverride      string
	enableReviews     bool
	// createErrTransform optionally rewrites the error returned from
	// POST /v1/sites BEFORE reportError formats it. Used by `bv
	// evidence` to surface the EV-E-8 distinctive 400 envelope when
	// the server's VALID_TEMPLATES set lags the CLI rollout. Other
	// callers leave this nil.
	createErrTransform func(error) error
}

var collectClientHostname = os.Hostname
var collectPublishInvocationMetadata = publishInvocationMetadata

// pushResult mirrors the JSON shape `bv push` emits on success. Templated
// commands surface the same shape so a caller piping `--json` sees a stable
// contract regardless of which command produced the site.
type pushResult struct {
	SiteID      string `json:"site_id"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	ManifestSHA string `json:"manifest_sha"`
	ExpiresAt   string `json:"expires_at"`
	Idempotent  bool   `json:"idempotent"`
	FileCount   int    `json:"file_count"`
	TotalBytes  int64  `json:"total_bytes"`
	Template    string `json:"template,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// runPushFlow drives the full POST /v1/sites → PUT → finalize sequence and
// writes output via g.w. Returns a process exit code (0 on success); the
// caller propagates that to os.Exit. errMsg is empty on success.
func runPushFlow(ctx context.Context, g globalContext, opts pushOptions) int {
	client, cfg, err := newClient(g)
	if err != nil {
		return reportError(g.w, err)
	}
	if canAutoRefreshToken(g, cfg) && shouldRefreshToken(cfg, time.Now()) {
		client, err = refreshInstallationToken(ctx, g, cfg)
		if err != nil {
			return reportError(g.w, err)
		}
	}
	if err := config.ValidateImageQuality(opts.imageQuality); err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	imageQuality, err := config.ResolveImageQuality(cfg, opts.imageQuality, 0)
	if err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}

	var buf bytes.Buffer
	info, err := tarbundle.BundleDir(opts.dir, &buf, tarbundle.Options{
		IncludeHidden: opts.includeHidden,
		ImageQuality:  imageQuality,
	})
	if err != nil {
		return reportError(g.w, fmt.Errorf("bundle %s: %w", opts.dir, err))
	}
	if !opts.skipGitleaksCheck {
		if err := checkBundleForSecrets(ctx, opts.dir, buf.Bytes()); err != nil {
			return reportError(g.w, err)
		}
	}
	pushProgress(g, 1, "Bundled", fmt.Sprintf("%d files (%d bytes)", info.FileCount, info.TotalBytes))

	clientHostname, _ := collectClientHostname()
	publishCommand, publishCWD := collectPublishInvocationMetadata()
	createReq := api.CreateSiteRequest{
		UploadID:       opts.uploadID,
		Template:       opts.template,
		SourcePath:     opts.sourcePath,
		ClientHostname: clientHostname,
		CLIVersion:     Version,
		PublishCommand: publishCommand,
		PublishCWD:     publishCWD,
		EnableReviews:  opts.enableReviews,
	}
	if opts.ttlSeconds > 0 {
		ttl := opts.ttlSeconds
		createReq.TTLSeconds = &ttl
	}
	var created api.CreateSiteResponse
	if err := client.Do(ctx, "POST", "/v1/sites", createReq, &created); err != nil {
		if api.IsUnauthenticated(err) && canAutoRefreshToken(g, cfg) {
			refreshedClient, refreshErr := refreshInstallationToken(ctx, g, cfg)
			if refreshErr != nil {
				return reportError(g.w, refreshErr)
			}
			client = refreshedClient
			err = client.Do(ctx, "POST", "/v1/sites", createReq, &created)
		}
		if err != nil {
			if opts.createErrTransform != nil {
				err = opts.createErrTransform(err)
			}
			return reportError(g.w, err)
		}
	}
	pushProgress(g, 2, "Provisioned", fmt.Sprintf("%s ready for upload", created.SiteID))
	if created.UploadMaxBytes > 0 && int64(buf.Len()) > created.UploadMaxBytes {
		return reportError(g.w, fmt.Errorf("bundle %s: tarbundle: bundle would exceed max_bytes=%d (tar size %d)", opts.dir, created.UploadMaxBytes, buf.Len()))
	}

	if err := putTar(ctx, created.UploadURL, buf.Bytes()); err != nil {
		return reportError(g.w, fmt.Errorf("upload tar: %w", err))
	}
	pushProgress(g, 3, "Uploaded", "bundle staged")

	var fin api.FinalizeResponse
	finalizeReq := api.FinalizeRequest{
		UploadID:       opts.uploadID,
		SourcePath:     opts.sourcePath,
		ClientHostname: clientHostname,
		CLIVersion:     Version,
		PublishCommand: publishCommand,
		PublishCWD:     publishCWD,
	}
	if err := client.Do(ctx, "POST", "/v1/sites/"+created.SiteID+"/finalize",
		finalizeReq, &fin); err != nil {
		if api.IsUnauthenticated(err) && canAutoRefreshToken(g, cfg) {
			refreshedClient, refreshErr := refreshInstallationToken(ctx, g, cfg)
			if refreshErr != nil {
				return reportError(g.w, refreshErr)
			}
			client = refreshedClient
			err = client.Do(ctx, "POST", "/v1/sites/"+created.SiteID+"/finalize",
				finalizeReq, &fin)
		}
		if err != nil {
			return reportError(g.w, err)
		}
	}

	res := pushResult{
		SiteID:      fin.SiteID,
		URL:         fin.URL,
		Status:      fin.Status,
		ManifestSHA: fin.ManifestSHA,
		ExpiresAt:   fin.ExpiresAt,
		Idempotent:  fin.Idempotent,
		FileCount:   info.FileCount,
		TotalBytes:  info.TotalBytes,
		Template:    opts.template,
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(res)
		return 0
	}
	pushProgress(g, 4, "Published", res.URL)
	writePushHumanResult(g, res)
	return 0
}

func publishInvocationMetadata() (string, string) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return publishShellJoin(os.Args), cwd
}

func publishShellJoin(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, publishShellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func publishShellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	safe := true
	for _, r := range arg {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '@', '%', '_', '+', '=', ':', ',', '.', '/', '-':
			continue
		}
		safe = false
		break
	}
	if safe {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func publishSourcePath(source string) string {
	if source == "" || source == "-" {
		return source
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return filepath.Clean(source)
	}
	return filepath.Clean(abs)
}

const pushProgressSteps = 4

// pushProgress renders one row of the multi-step push pipeline. The plain
// `[####----]` bar is preserved verbatim so existing log assertions keep
// matching; in a TTY the bar is colored cyan and the step label bold so
// the eye lands on the current verb at a glance.
func pushProgress(g globalContext, step int, label, detail string) {
	barWidth := 20
	filled := step * barWidth / pushProgressSteps
	bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
	st := g.w.StderrStyler()
	stepCol := st.Dim(fmt.Sprintf("[%d/%d]", step, pushProgressSteps))
	barCol := st.Cyan("[" + bar + "]")
	labelCol := st.Bold(label)
	if detail == "" {
		g.w.Progress("%s %s %s", stepCol, barCol, labelCol)
		return
	}
	g.w.Progress("%s %s %s: %s", stepCol, barCol, labelCol, detail)
}

func writePushHumanResult(g globalContext, res pushResult) {
	g.w.Success("Published site")
	g.w.Human("  Open URL:   %s", g.w.StdoutStyler().Cyan(res.URL))
	g.w.Section("Metadata")
	g.w.KV("Site ID", res.SiteID)
	g.w.KV("Status", styledStatus(g, res.Status))
	g.w.KV("Manifest", g.w.StdoutStyler().Dim(res.ManifestSHA))
	g.w.KVf("Files", "%d", res.FileCount)
	g.w.KV("Size", humanByteSize(res.TotalBytes))
	if res.Template != "" {
		g.w.KV("Template", res.Template)
	}
	if res.ExpiresAt != "" {
		g.w.KV("Expires", res.ExpiresAt)
	}
	if res.Idempotent {
		g.w.Hint("Idempotent retry: no bytes re-extracted")
	}
}

// humanByteSize renders a byte count both as the canonical "%d bytes"
// (preserved for tests + log scrapers) and a human-friendly suffix when
// the value crosses a kilobyte. Sub-KiB values stay bare so a 47-byte
// publish doesn't read as "47 bytes (47 B)".
func humanByteSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	const (
		kib = 1024
		mib = 1024 * 1024
		gib = 1024 * 1024 * 1024
	)
	switch {
	case n < mib:
		return fmt.Sprintf("%d bytes (%.1f KiB)", n, float64(n)/kib)
	case n < gib:
		return fmt.Sprintf("%d bytes (%.1f MiB)", n, float64(n)/mib)
	default:
		return fmt.Sprintf("%d bytes (%.2f GiB)", n, float64(n)/gib)
	}
}

// styledStatus colors well-known status strings so a green "active"
// reads as healthy at a glance. Unknown values pass through unchanged.
func styledStatus(g globalContext, status string) string {
	s := g.w.StdoutStyler()
	switch strings.ToLower(status) {
	case "active":
		return s.Green(status)
	case "expired", "deleted":
		return s.Red(status)
	case "creating", "uploading", "pending", "local":
		return s.Yellow(status)
	default:
		return status
	}
}
