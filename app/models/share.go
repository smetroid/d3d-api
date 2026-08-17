package models

import "time"

type Share struct {
	Id        string    `db:"id" json:"id"`
	DagId     string    `db:"dag_id" json:"dagId"`
	Jti       string    `db:"jti" json:"jti"`
	Role      string    `db:"role" json:"role"` // "view" | "edit"
	AnonName  string    `db:"anon_name" json:"anonName"`
	CreatedBy string    `db:"created_by" json:"createdBy"`
	ExpiresAt time.Time `db:"expires_at" json:"expiresAt"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

type ShareDenylist struct {
	Jti       string    `db:"jti" json:"id"`
	RevokedAt time.Time `db:"revoked_at" json:"revokedAt"`
}

type CreateShareRequest struct {
	Role    string `json:"role"`    // "view" | "edit"
	ExpDays int    `json:"expDays"` // 0 defaults to 7
}

type CreateShareResponse struct {
	Status string `json:"status"`
	Token  string `json:"token"`
	Jti    string `json:"jti"`
	Role   string `json:"role"`
}

type ExchangeShareResponse struct {
	Status   string `json:"status"`
	DagId    string `json:"dagId"`
	Role     string `json:"role"`
	Jti      string `json:"jti"`
	AnonName string `json:"anonName"`
}
