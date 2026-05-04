package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

func checkBundleForSecrets(ctx context.Context, sourceDir string, tarBytes []byte) error {
	detector, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return fmt.Errorf("initialize gitleaks check: %w", err)
	}
	detector.Redact = 100

	findings, err := detectTarBundle(ctx, detector, tarBytes)
	if err != nil {
		return fmt.Errorf("scan bundle with gitleaks: %w", err)
	}
	if len(findings) == 0 {
		return nil
	}
	return formatGitleaksBlockError(sourceDir, findings)
}

func detectTarBundle(ctx context.Context, detector *detect.Detector, tarBytes []byte) ([]report.Finding, error) {
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	var findings []report.Finding
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		fileFindings, err := detector.DetectSource(ctx, &sources.File{
			Content: bytes.NewReader(content),
			Path:    hdr.Name,
			Config:  &detector.Config,
		})
		if err != nil {
			return nil, err
		}
		findings = append(findings, fileFindings...)
	}
	return findings, nil
}

func formatGitleaksBlockError(sourceDir string, findings []report.Finding) error {
	limit := min(len(findings), 5)
	lines := []string{
		fmt.Sprintf("gitleaks detected %d potential secret(s) in %s", len(findings), sourceDir),
		"bv push was blocked before creating or uploading a site.",
	}
	for i := 0; i < limit; i++ {
		finding := findings[i]
		location := finding.File
		if finding.StartLine > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.StartLine)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", location, finding.RuleID))
	}
	if len(findings) > limit {
		lines = append(lines, fmt.Sprintf("- ... %d more", len(findings)-limit))
	}
	lines = append(lines, "Remove the secret or rerun with --skip-gitleaks-check if you intentionally want to upload it.")
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}
