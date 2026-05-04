// `bv push <dir>` is the primary CLI command. Steps:
//
//  1. Generate a random upload_id (or accept --upload-id for explicit retry).
//  2. POST /v1/sites — gets back a signed PUT URL + upload_max_bytes cap.
//  3. Bundle <dir> into a tar in memory (or a temp file for big sites).
//  4. PUT the tar to the signed URL with Content-Length matching the bundle.
//  5. POST /v1/sites/{id}/finalize — server untars + computes manifest_sha.
//  6. Emit JSON, or in human mode show progress on stderr and print the
//     published URL plus structured site metadata on stdout.
//
// Idempotency: the upload_id is the binding. A retry with the SAME upload_id
// re-uses the existing site_id; if the prior call already finalized, the
// finalize is a no-op idempotent ack. The CLI does NOT derive upload_id from
// content — bare `bv push <dir>` mints a fresh random id each invocation, so
// two back-to-back pushes of the same directory produce TWO sites. To re-use
// a half-finished provision after a transient error, pass --upload-id with
// the value the failed run printed. (Content-derived upload_ids are an
// ergonomic candidate for v1.x; doing it well requires hashing the bundle
// before the create call, which interacts with the upload_max_bytes
// negotiation that today happens server-side first.)
//
// Heartbeat: for bundles whose upload would take >30min the CLI fires a
// heartbeat per minute against the server. We do NOT spin up a goroutine
// at MVP — the upload itself is sequential and 100MB/30min implies the user
// already has a problem. v1.x adds the heartbeat goroutine if metrics show
// long-tail uploads.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runPush(ctx context.Context, g globalContext, args []string) int {
	fs, flags := newCLIFlagSet("push")
	uploadIDFlag := flags.String("upload-id")
	ttlFlag := flags.Int64("ttl-seconds")
	includeHidden := flags.Bool("include-hidden")
	skipGitleaksCheck := flags.Bool("skip-gitleaks-check")
	imageQuality := flags.Int("image-quality")
	modeFlag := flags.String("mode")
	if err := fs.Parse(args); err != nil {
		return handleFlagParseError(g, "push", err)
	}
	pos := fs.Args()
	if len(pos) < 1 {
		g.w.Error(toErrorEnvelope(usageError("push")))
		return 2
	}
	dir := pos[0]

	// Generate the upload_id BEFORE bundling — the CLI's POST /v1/sites
	// call carries it. A retry with --upload-id passes through unchanged.
	uploadID := *uploadIDFlag
	if uploadID == "" {
		var err error
		uploadID, err = newUploadID()
		if err != nil {
			return reportError(g.w, fmt.Errorf("generate upload_id: %w", err))
		}
	}
	return runPushFlowForMode(ctx, g, pushOptions{
		dir:               dir,
		sourcePath:        publishSourcePath(dir),
		uploadID:          uploadID,
		ttlSeconds:        *ttlFlag,
		includeHidden:     *includeHidden,
		skipGitleaksCheck: *skipGitleaksCheck,
		imageQuality:      *imageQuality,
		modeOverride:      *modeFlag,
	})
}

// putTar uploads the tar bytes to a signed PUT URL. Sets Content-Length
// explicitly so the signed-URL Content-Length-Range gate sees the right
// value (default Go HTTP would chunk the body when the body is a Reader).
//
// Uses a dedicated client with a generous-but-finite Timeout so a stalled
// R2 upload eventually fails with a network error rather than hanging the
// CLI process forever. http.DefaultClient has Timeout=0 (no deadline) and
// the parent ctx only fires on user signal, not on a dead TCP connection.
func putTar(ctx context.Context, signedURL string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", signedURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/x-tar")
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT failed: %s: %s", resp.Status, truncate(string(buf), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// newUploadID generates a 24-char hex string suitable for use as an
// upload_id. The server alphabet is [A-Za-z0-9_-]{1,128} — 24 hex chars
// fits comfortably and gives ~96 bits of entropy (collision-resistant).
func newUploadID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	// Prefix with 'u-' so an upload_id is recognizable in logs and not
	// confused with a site_id (which never contains a dash).
	return "u-" + hex.EncodeToString(raw[:]), nil
}
