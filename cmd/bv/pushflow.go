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
	"strings"
	"time"

	"github.com/htxryan/butverify/internal/api"
	"github.com/htxryan/butverify/pkg/tarbundle"
)

// pushOptions bundles the parameters of a single push pipeline run.
type pushOptions struct {
	dir           string
	uploadID      string
	ttlSeconds    int64 // 0 = use server default
	template      string
	includeHidden bool
	// createErrTransform optionally rewrites the error returned from
	// POST /v1/sites BEFORE reportError formats it. Used by `bv
	// evidence` to surface the EV-E-8 distinctive 400 envelope when
	// the server's VALID_TEMPLATES set lags the CLI rollout. Other
	// callers leave this nil.
	createErrTransform func(error) error
}

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
	createReq := api.CreateSiteRequest{UploadID: opts.uploadID, Template: opts.template}
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
	pushProgress(g, 1, "Provisioned", fmt.Sprintf("%s ready for upload", created.SiteID))

	var buf bytes.Buffer
	info, err := tarbundle.BundleDir(opts.dir, &buf, tarbundle.Options{
		MaxBytes:      created.UploadMaxBytes,
		IncludeHidden: opts.includeHidden,
	})
	if err != nil {
		return reportError(g.w, fmt.Errorf("bundle %s: %w", opts.dir, err))
	}
	pushProgress(g, 2, "Bundled", fmt.Sprintf("%d files (%d bytes)", info.FileCount, info.TotalBytes))

	if err := putTar(ctx, created.UploadURL, buf.Bytes()); err != nil {
		return reportError(g.w, fmt.Errorf("upload tar: %w", err))
	}
	pushProgress(g, 3, "Uploaded", "bundle staged")

	var fin api.FinalizeResponse
	if err := client.Do(ctx, "POST", "/v1/sites/"+created.SiteID+"/finalize",
		api.FinalizeRequest{UploadID: opts.uploadID}, &fin); err != nil {
		if api.IsUnauthenticated(err) && canAutoRefreshToken(g, cfg) {
			refreshedClient, refreshErr := refreshInstallationToken(ctx, g, cfg)
			if refreshErr != nil {
				return reportError(g.w, refreshErr)
			}
			client = refreshedClient
			err = client.Do(ctx, "POST", "/v1/sites/"+created.SiteID+"/finalize",
				api.FinalizeRequest{UploadID: opts.uploadID}, &fin)
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

const pushProgressSteps = 4

func pushProgress(g globalContext, step int, label, detail string) {
	barWidth := 20
	filled := step * barWidth / pushProgressSteps
	bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
	if detail == "" {
		g.w.Status("[%d/%d] [%s] %s", step, pushProgressSteps, bar, label)
		return
	}
	g.w.Status("[%d/%d] [%s] %s: %s", step, pushProgressSteps, bar, label, detail)
}

func writePushHumanResult(g globalContext, res pushResult) {
	g.w.Human("Published site")
	g.w.Human("  Open URL:   %s", res.URL)
	g.w.Human("\n")
	g.w.Human("Metadata")
	g.w.Human("  Site ID:    %s", res.SiteID)
	g.w.Human("  Status:     %s", res.Status)
	g.w.Human("  Manifest:   %s", res.ManifestSHA)
	g.w.Human("  Files:      %d", res.FileCount)
	g.w.Human("  Size:       %d bytes", res.TotalBytes)
	if res.Template != "" {
		g.w.Human("  Template:   %s", res.Template)
	}
	if res.ExpiresAt != "" {
		g.w.Human("  Expires:    %s", res.ExpiresAt)
	}
	if res.Idempotent {
		g.w.Human("\n")
		g.w.Human("Idempotent retry: no bytes re-extracted")
	}
}
