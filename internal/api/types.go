// API request + response types — Go twin of the TypeScript ApiResponse
// shapes in apps/control-plane/src/routes/sites.ts. Keep these in lockstep
// with the server; OpenAPI spec at docs/openapi.yaml is the source of truth.

package api

// WhoamiResponse is the GET /v1/auth/whoami payload.
type WhoamiResponse struct {
	TenantID       string `json:"tenant_id"`
	InstallationID int64  `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
	AccountType    string `json:"account_type"`
	ExpiresAt      string `json:"expires_at"`
}

// LoginResponse is the POST /v1/auth/login payload. The server exchanges
// a GitHub user token for a freshly-minted installation token; the CLI
// persists the result via bv login so subsequent commands authenticate
// with `Token` against the control plane.
type LoginResponse struct {
	Token          string `json:"token"`
	ExpiresAt      string `json:"expires_at"`
	TenantID       string `json:"tenant_id"`
	AccountLogin   string `json:"account_login"`
	InstallationID int64  `json:"installation_id"`
	AccountType    string `json:"account_type"`
}

// CreateSiteRequest is the POST /v1/sites body.
type CreateSiteRequest struct {
	UploadID       string `json:"upload_id"`
	TTLSeconds     *int64 `json:"ttl_seconds,omitempty"`
	SourcePath     string `json:"source_path,omitempty"`
	ClientHostname string `json:"client_hostname,omitempty"`
	CLIVersion     string `json:"cli_version,omitempty"`
	PublishCommand string `json:"publish_command,omitempty"`
	PublishCWD     string `json:"publish_cwd,omitempty"`
	// Template is set when the CLI is invoking a templated artifact path; empty for a regular `bv push`. Server uses
	// this to bill the request against the templated-site fairness counter
	// (closes O-3) and stamp the sites row's `template` column for analytics.
	Template string `json:"template,omitempty"`
}

// CreateSiteResponse is the POST /v1/sites payload.
type CreateSiteResponse struct {
	SiteID             string `json:"site_id"`
	URL                string `json:"url"`
	ExpiresAt          string `json:"expires_at"`
	UploadToken        string `json:"upload_token"`
	ManifestURL        string `json:"manifest_url"`
	Status             string `json:"status"`
	Idempotent         bool   `json:"idempotent"`
	UploadURL          string `json:"upload_url"`
	UploadMaxBytes     int64  `json:"upload_max_bytes"`
	UploadURLExpiresAt string `json:"upload_url_expires_at"`
	// Template echoes the server-stored value (or "" for
	// a non-templated push). Surfaces what was actually stamped on the row
	// so an idempotent retry that passed a different template can detect
	// the divergence at the boundary rather than inside D1 reads.
	Template string `json:"template,omitempty"`
}

// SiteSummary is one entry in GET /v1/sites and the body of GET /v1/sites/{id}.
type SiteSummary struct {
	SiteID       string `json:"site_id"`
	TenantID     string `json:"tenant_id"`
	Status       string `json:"status"`
	URL          string `json:"url"`
	ManifestURL  string `json:"manifest_url"`
	ExpiresAt    string `json:"expires_at"`
	PinnedAt     string `json:"pinned_at"`
	BytesUsed    int64  `json:"bytes_used"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LastPushedAt string `json:"last_pushed_at"`
}

// ListSitesResponse is the GET /v1/sites payload.
type ListSitesResponse struct {
	Sites []SiteSummary `json:"sites"`
}

// FinalizeRequest is the POST /v1/sites/{id}/finalize body.
type FinalizeRequest struct {
	UploadID       string `json:"upload_id"`
	ManifestSHA    string `json:"manifest_sha,omitempty"`
	SourcePath     string `json:"source_path,omitempty"`
	ClientHostname string `json:"client_hostname,omitempty"`
	CLIVersion     string `json:"cli_version,omitempty"`
	PublishCommand string `json:"publish_command,omitempty"`
	PublishCWD     string `json:"publish_cwd,omitempty"`
}

// FinalizeResponse is the POST /v1/sites/{id}/finalize payload.
type FinalizeResponse struct {
	SiteID       string `json:"site_id"`
	Status       string `json:"status"`
	URL          string `json:"url"`
	ManifestURL  string `json:"manifest_url"`
	ExpiresAt    string `json:"expires_at"`
	ManifestSHA  string `json:"manifest_sha"`
	LastPushedAt string `json:"last_pushed_at"`
	Idempotent   bool   `json:"idempotent"`
}

// DeleteSiteResponse is the DELETE /v1/sites/{id} payload.
type DeleteSiteResponse struct {
	SiteID string `json:"site_id"`
	Status string `json:"status"`
}

// PinResponse is the POST /v1/sites/{id}/{pin,unpin} payload.
type PinResponse struct {
	SiteID    string `json:"site_id"`
	Status    string `json:"status"`
	PinnedAt  string `json:"pinned_at"`
	ExpiresAt string `json:"expires_at"`
}

// HeartbeatRequest is the POST /v1/sites/{id}/heartbeat body.
type HeartbeatRequest struct {
	UploadID string `json:"upload_id"`
}

// HeartbeatResponse is the POST /v1/sites/{id}/heartbeat payload.
type HeartbeatResponse struct {
	SiteID      string `json:"site_id"`
	Status      string `json:"status"`
	HeartbeatAt string `json:"heartbeat_at"`
}

// FilesListResponse is the GET /v1/sites/{id}/files payload.
type FilesListResponse struct {
	SiteID     string         `json:"site_id"`
	FileCount  int            `json:"file_count"`
	TotalBytes int64          `json:"total_bytes"`
	Files      []ManifestFile `json:"files"`
}

// ManifestFile mirrors a single entry in the manifest's `files` array.
type ManifestFile struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

// ReviewSummary is one entry in GET /v1/reviews. Mirrors
// ListReviewsResponse.reviews in apps/control-plane/src/routes/reviews.ts.
// AcknowledgedAt is *string so a server-sent JSON `null` (unacknowledged)
// round-trips as `null` rather than being silently dropped via omitempty.
type ReviewSummary struct {
	ReviewID        string  `json:"review_id"`
	SiteID          string  `json:"site_id"`
	ReviewerLogin   string  `json:"reviewer_login"`
	SubmittedAt     string  `json:"submitted_at"`
	AcknowledgedAt  *string `json:"acknowledged_at"`
	AnnotationCount int     `json:"annotation_count"`
	Status          string  `json:"status"`
}

// ListReviewsResponse is the GET /v1/reviews payload.
type ListReviewsResponse struct {
	Reviews []ReviewSummary `json:"reviews"`
}

// Annotation mirrors one annotation row returned by GET /v1/reviews/:id.
// Optional columns are *T so JSON `null` round-trips as nil rather than
// the zero value (a 0.0 region_x is not the same as "no region").
type Annotation struct {
	AnnotationID string   `json:"annotation_id"`
	Type         string   `json:"type"`
	ItemIndex    *int     `json:"item_index"`
	Comment      string   `json:"comment"`
	RegionShape  *string  `json:"region_shape"`
	RegionX      *float64 `json:"region_x"`
	RegionY      *float64 `json:"region_y"`
	RegionWidth  *float64 `json:"region_width"`
	RegionHeight *float64 `json:"region_height"`
	TextScope    *string  `json:"text_scope"`
	CharStart    *int     `json:"char_start"`
	CharEnd      *int     `json:"char_end"`
	TextSnippet  *string  `json:"text_snippet"`
	CreatedAt    string   `json:"created_at"`
}

// GetReviewResponse is the GET /v1/reviews/:id payload — full review +
// annotations.
type GetReviewResponse struct {
	ReviewID        string       `json:"review_id"`
	SiteID          string       `json:"site_id"`
	ReviewerLogin   string       `json:"reviewer_login"`
	SubmittedAt     string       `json:"submitted_at"`
	AcknowledgedAt  *string      `json:"acknowledged_at"`
	AnnotationCount int          `json:"annotation_count"`
	Status          string       `json:"status"`
	Annotations     []Annotation `json:"annotations"`
}

// AcknowledgeReviewResponse is the PATCH /v1/reviews/:id/acknowledge payload.
// `already_acknowledged` distinguishes a fresh ack from an idempotent retry
// so the CLI can keep its exit code at 0 (per EV2-S-2) while still telling
// the human which path they took.
type AcknowledgeReviewResponse struct {
	ReviewID            string `json:"review_id"`
	Status              string `json:"status"`
	AcknowledgedAt      string `json:"acknowledged_at"`
	AlreadyAcknowledged bool   `json:"already_acknowledged"`
}

// RequestReviewBody is the POST /v1/sites/:id/review-requests body.
type RequestReviewBody struct {
	ReviewerLogin string `json:"reviewer_login"`
}

// RequestReviewSuccess is the 201 success body for a review request.
type RequestReviewSuccess struct {
	Requested     bool   `json:"requested"`
	ReviewerLogin string `json:"reviewer_login"`
	Notification  string `json:"notification"`
}

// RequestReviewUnresolvable is the 422 body when the reviewer's email
// can't be resolved. The control plane returns it directly (NOT inside
// the standard error envelope) so the CLI surfaces site_url for manual
// sharing per EV2-N-7.
type RequestReviewUnresolvable struct {
	Requested bool   `json:"requested"`
	Error     string `json:"error"`
	SiteURL   string `json:"site_url"`
}
