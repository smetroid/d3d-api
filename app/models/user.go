package models

import "time"

type User struct {
	Id           string    `gorethink:"id" json:"id"`
	Username     string    `gorethink:"username" json:"username"`
	PasswordHash string    `gorethink:"password_hash" json:"-"`
	CreatedAt    time.Time `gorethink:"created_at" json:"createdAt"`
}
