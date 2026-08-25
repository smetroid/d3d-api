package models

import "time"

type Company struct {
	Id        string    `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	CreatedBy string    `db:"created_by" json:"createdBy"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// Membership records a user's belonging to a company.
// UserId is a username string (not a UUID FK) so LDAP users who have no row
// in the users table can still be members.
type Membership struct {
	UserId    string `db:"user_id" json:"userId"`
	CompanyId string `db:"company_id" json:"companyId"`
}

// Group maps to the user_groups table (renamed to avoid the PG11+ reserved
// word GROUPS). ExternalRef holds the LDAP/AD group DN for externally-derived
// groups; it is empty for natively-created groups.
type Group struct {
	Id          string    `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	CompanyId   string    `db:"company_id" json:"companyId"`
	ExternalRef string    `db:"external_ref" json:"externalRef,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

type GroupMember struct {
	GroupId string `db:"group_id" json:"groupId"`
	UserId  string `db:"user_id" json:"userId"`
}

// ElementShare is a snapshot share of a node/edge/cluster subgraph.
// Cluster contains the serialized graphlib subgraph JSON: {nodes:[…], edges:[…]}.
// Jti is only populated for public link shares (audience_kind = "public") and
// is used for the same denylist revocation flow as diagram share links.
type ElementShare struct {
	Id           string    `db:"id" json:"id"`
	Title        string    `db:"title" json:"title"`
	Type         string    `db:"type" json:"type"`                  // "node" | "edge" | "cluster"
	RootIds      []string  `db:"root_ids" json:"rootIds"`
	Cluster      string    `db:"cluster" json:"cluster"`            // graphlib subgraph JSON
	AudienceKind string    `db:"audience_kind" json:"audienceKind"` // "public" | "user" | "company" | "group"
	AudienceIds  []string  `db:"audience_ids" json:"audienceIds"`
	Role         string    `db:"role" json:"role"`                  // "view" | "edit"
	CreatedBy    string    `db:"created_by" json:"createdBy"`
	SourceDagId  string    `db:"source_dag_id" json:"sourceDagId,omitempty"`
	ExpiresAt    time.Time `db:"expires_at" json:"expiresAt,omitempty"`
	Revoked      bool      `db:"revoked" json:"revoked"`
	Catalog      bool      `db:"catalog" json:"catalog"`
	Tags         []string  `db:"tags" json:"tags"`
	ImportedBy   []string  `db:"imported_by" json:"importedBy"`
	Jti          string    `db:"jti" json:"jti,omitempty"`
	AnonName     string    `db:"anon_name" json:"anonName,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
}

// AudienceSpec describes who a share is visible to.
type AudienceSpec struct {
	Kind string   `json:"kind"` // "public" | "user" | "company" | "group"
	Ids  []string `json:"ids"`  // usernames, companyIds, or groupIds; empty for "public"
}

// CreateElementShareRequest is the body for POST /dag/:dag/elements/shares.
// Depth controls the cluster closure: 0 = element+descendants only,
// 1 = +1 hop neighbors, nil = whole connected component.
type CreateElementShareRequest struct {
	RootIds  []string     `json:"rootIds"`
	Depth    *int         `json:"depth"`
	Audience AudienceSpec `json:"audience"`
	Role     string       `json:"role"`
	ExpDays  int          `json:"expDays"`
	Catalog  bool         `json:"catalog"`
	Tags     []string     `json:"tags"`
	Title    string       `json:"title"`
}

type ElementShareSummary struct {
	Id           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	RootIds      []string  `json:"rootIds"`
	AudienceKind string    `json:"audienceKind"`
	Role         string    `json:"role"`
	CreatedBy    string    `json:"createdBy"`
	SourceDagId  string    `json:"sourceDagId,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	Catalog      bool      `json:"catalog"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"createdAt"`
}

// CatalogRow is the raw DB record from ListCatalogShares. The controller
// derives NodeCount, EdgeCount, and Token before serving the CatalogEntry.
type CatalogRow struct {
	Id        string
	Title     string
	CreatedBy string
	RootIds   []string
	Tags      []string
	Cluster   string
	Jti       string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CatalogEntry is the public-facing catalog item returned by GET /catalog.
type CatalogEntry struct {
	Id        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedBy string    `json:"createdBy"`
	RootIds   []string  `json:"rootIds"`
	NodeCount int       `json:"nodeCount"`
	EdgeCount int       `json:"edgeCount"`
	Token     string    `json:"token"`
	Tags      []string  `json:"tags"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type ElementShareResponse struct {
	Status string       `json:"status"`
	Share  ElementShare `json:"share"`
}

type ElementShareListResponse struct {
	Status string                `json:"status"`
	Shares []ElementShareSummary `json:"shares"`
}
