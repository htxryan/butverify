package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/htxryan/butverify/internal/config"
)

func TestLocalSiteServerServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello local"), 0o644); err != nil {
		t.Fatal(err)
	}
	server, err := startLocalSiteServer(dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = server.Close() }()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello local") {
		t.Fatalf("body=%q", body)
	}
}

func TestStageLocalSiteExcludesHiddenByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOP-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged, info, cleanup, err := stageLocalSite(dir, false, 0)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	defer cleanup()
	if info.FileCount != 1 {
		t.Fatalf("file count=%d, want 1", info.FileCount)
	}
	if _, err := os.Stat(filepath.Join(staged, ".env")); !os.IsNotExist(err) {
		t.Fatalf("hidden file should not be staged by default: %v", err)
	}
	server, err := startLocalSiteServer(staged)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = server.Close() }()
	resp, err := http.Get(server.URL + ".env")
	if err != nil {
		t.Fatalf("get hidden: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("hidden file status=%d body=%q, want 404", resp.StatusCode, body)
	}
}

func TestStageLocalSiteIncludesHiddenWhenRequested(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOP-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged, info, cleanup, err := stageLocalSite(dir, true, 0)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	defer cleanup()
	if info.FileCount != 2 {
		t.Fatalf("file count=%d, want 2", info.FileCount)
	}
	if data, err := os.ReadFile(filepath.Join(staged, ".env")); err != nil || string(data) != "TOP-SECRET" {
		t.Fatalf("hidden file not staged with includeHidden: data=%q err=%v", data, err)
	}
}

func TestStageLocalSiteOptimizesImages(t *testing.T) {
	dir := t.TempDir()
	original := writeCLIJPEGFixture(t, filepath.Join(dir, "photo.jpg"), 100)
	staged, _, cleanup, err := stageLocalSite(dir, false, 40)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	defer cleanup()
	optimized, err := os.ReadFile(filepath.Join(staged, "photo.jpg"))
	if err != nil {
		t.Fatalf("read staged photo: %v", err)
	}
	if len(optimized) >= len(original) {
		t.Fatalf("staged photo was not optimized: got %d original %d", len(optimized), len(original))
	}
}

func TestPushModeLocalUsesConfiguredImageQuality(t *testing.T) {
	localServer := withFakeLocalServer(t)
	dir := t.TempDir()
	original := writeCLIJPEGFixture(t, filepath.Join(dir, "photo.jpg"), 100)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<img src=photo.jpg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedConfigPath(t)
	if err := config.Save(&config.Config{Mode: config.ModeLocal, ImageQuality: 40}); err != nil {
		t.Fatal(err)
	}
	w, stdout, _ := newJSONWriter(t)
	rc := runPush(context.Background(), globalContext{w: w}, []string{dir})
	if rc != 0 {
		t.Fatalf("push rc=%d stdout=%s", rc, stdout.String())
	}
	optimized := localServer.Photo
	if len(optimized) == 0 {
		t.Fatal("fake local server did not capture staged photo")
	}
	if len(optimized) >= len(original) {
		t.Fatalf("configured local photo was not optimized: got %d original %d", len(optimized), len(original))
	}
}

func TestLocalSiteOutputPathRejectsTraversal(t *testing.T) {
	if _, err := localSiteOutputPath(t.TempDir(), "../escape.html"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

type fakeLocalServerCapture struct {
	Root      string
	IndexHTML string
	Photo     []byte
}

func withFakeLocalServer(t *testing.T) *fakeLocalServerCapture {
	t.Helper()
	oldStart := startLocalSiteServer
	oldWait := waitForLocalSite
	capture := &fakeLocalServerCapture{}
	startLocalSiteServer = func(r string) (*localSiteServer, error) {
		capture.Root = r
		if data, err := os.ReadFile(filepath.Join(r, "index.html")); err == nil {
			capture.IndexHTML = string(data)
		}
		if data, err := os.ReadFile(filepath.Join(r, "photo.jpg")); err == nil {
			capture.Photo = data
		}
		return &localSiteServer{URL: "http://127.0.0.1:12345/"}, nil
	}
	waitForLocalSite = func(context.Context, *localSiteServer) error { return nil }
	t.Cleanup(func() {
		startLocalSiteServer = oldStart
		waitForLocalSite = oldWait
	})
	return capture
}
