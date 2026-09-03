package models

import "time"

type User struct {
	Id           string    `db:"id" json:"id"`
	Username     string    `db:"username" json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	Provider     string    `db:"provider" json:"provider"`
	ProviderID   string    `db:"provider_id" json:"providerId"`
	Email        string    `db:"email" json:"email"`
	DisplayName  string    `db:"display_name" json:"displayName"`
}
