package models

import "time"

type Share struct {
	Id        string    `gorethink:"id" json:"id"`
	DagId     string    `gorethink:"dag_id" json:"dagId"`
	Jti       string    `gorethink:"jti" json:"jti"`
	Role      string    `gorethink:"role" json:"role"` // "view" | "edit"
	CreatedBy string    `gorethink:"created_by" json:"createdBy"`
	ExpiresAt time.Time `gorethink:"expires_at" json:"expiresAt"`
	CreatedAt time.Time `gorethink:"created_at" json:"createdAt"`
}

type ShareDenylist struct {
	Jti       string    `gorethink:"id" json:"id"`
	RevokedAt time.Time `gorethink:"revoked_at" json:"revokedAt"`
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
