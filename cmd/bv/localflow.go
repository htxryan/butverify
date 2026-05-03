package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/htxryan/butverify/internal/config"
	"github.com/htxryan/butverify/pkg/tarbundle"
)

type localSiteServer struct {
	URL string
	srv *http.Server
}

func (s *localSiteServer) Close() error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

var startLocalSiteServer = func(root string) (*localSiteServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(root))}
	go func() {
		_ = srv.Serve(ln)
	}()
	return &localSiteServer{URL: "http://" + ln.Addr().String() + "/", srv: srv}, nil
}

var waitForLocalSite = func(ctx context.Context, _ *localSiteServer) error {
	<-ctx.Done()
	return ctx.Err()
}

func runPushFlowForMode(ctx context.Context, g globalContext, opts pushOptions) int {
	mode, err := resolvedPublishMode(g, opts.modeOverride)
	if err != nil {
		if opts.modeOverride != "" {
			g.w.Error(toErrorEnvelope(err))
			return 2
		}
		return reportError(g.w, err)
	}
	if mode == config.ModeLocal {
		return runLocalPushFlow(ctx, g, opts)
	}
	return runPushFlow(ctx, g, opts)
}

func runLocalPushFlow(ctx context.Context, g globalContext, opts pushOptions) int {
	c, err := loadModeConfig()
	if err != nil {
		return reportError(g.w, err)
	}
	imageQuality, err := config.ResolveImageQuality(c, opts.imageQuality, 0)
	if err != nil {
		g.w.Error(toErrorEnvelope(err))
		return 2
	}
	serveDir, info, cleanup, err := stageLocalSite(opts.dir, opts.includeHidden, imageQuality)
	if err != nil {
		return reportError(g.w, fmt.Errorf("bundle %s: %w", opts.dir, err))
	}
	defer cleanup()
	server, err := startLocalSiteServer(serveDir)
	if err != nil {
		return reportError(g.w, fmt.Errorf("start local server: %w", err))
	}
	defer func() { _ = server.Close() }()

	res := pushResult{
		SiteID:     localSiteID(opts),
		URL:        server.URL,
		Status:     config.ModeLocal,
		FileCount:  info.FileCount,
		TotalBytes: info.TotalBytes,
		Template:   opts.template,
		Mode:       config.ModeLocal,
	}
	if g.w.IsJSON() {
		_ = g.w.JSON(res)
	} else {
		writeLocalHumanResult(g, res, opts)
	}
	if err := waitForLocalSite(ctx, server); err != nil && err != context.Canceled {
		return reportError(g.w, err)
	}
	return 0
}

func stageLocalSite(src string, includeHidden bool, imageQuality int) (string, tarbundle.BundleInfo, func(), error) {
	bundle, err := os.CreateTemp("", "bv-local-site-*.tar")
	if err != nil {
		return "", tarbundle.BundleInfo{}, func() {}, fmt.Errorf("stage local site temp bundle: %w", err)
	}
	defer func() { _ = os.Remove(bundle.Name()) }()
	defer func() { _ = bundle.Close() }()

	info, err := tarbundle.BundleDir(src, bundle, tarbundle.Options{IncludeHidden: includeHidden, ImageQuality: imageQuality})
	if err != nil {
		return "", tarbundle.BundleInfo{}, func() {}, err
	}
	if _, err := bundle.Seek(0, io.SeekStart); err != nil {
		return "", tarbundle.BundleInfo{}, func() {}, fmt.Errorf("stage local site seek bundle: %w", err)
	}
	dir, err := os.MkdirTemp("", "bv-local-site-")
	if err != nil {
		return "", tarbundle.BundleInfo{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := extractLocalSiteTar(bundle, dir); err != nil {
		cleanup()
		return "", tarbundle.BundleInfo{}, func() {}, err
	}
	return dir, info, cleanup, nil
}

func extractLocalSiteTar(bundle io.Reader, dstRoot string) error {
	r := tar.NewReader(bundle)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("stage local site read tar: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		dst := filepath.Join(dstRoot, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("stage local site mkdir %s: %w", h.Name, err)
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("stage local site create %s: %w", h.Name, err)
		}
		_, copyErr := io.Copy(out, r)
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("stage local site copy %s: %w", h.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("stage local site close %s: %w", h.Name, closeErr)
		}
	}
	return nil
}

func writeLocalHumanResult(g globalContext, res pushResult, opts pushOptions) {
	g.w.Human("Serving local site")
	g.w.Human("  Open URL:   %s", res.URL)
	g.w.Human("\n")
	g.w.Human("Metadata")
	g.w.Human("  Site ID:    %s", res.SiteID)
	g.w.Human("  Status:     %s", res.Status)
	g.w.Human("  Files:      %d", res.FileCount)
	g.w.Human("  Size:       %d bytes", res.TotalBytes)
	if res.Template != "" {
		g.w.Human("  Template:   %s", res.Template)
	}
	if opts.sourcePath != "" {
		g.w.Human("  Source:     %s", opts.sourcePath)
	}
	g.w.Status("Serving %s locally until interrupted", opts.dir)
}

func localSiteID(opts pushOptions) string {
	base := opts.uploadID
	if base == "" {
		base = opts.template
	}
	if base == "" {
		base = "site"
	}
	base = strings.TrimPrefix(base, "u-")
	if len(base) > 12 {
		base = base[:12]
	}
	return "local-" + base
}
