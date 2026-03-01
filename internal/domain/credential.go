package domain

import (
	"time"
)

// Credential stores password and login information for a user
type Credential struct {
	UserID            string     `json:"user_id"`
	PasswordHash      string     `json:"-"` // Never expose this in JSON
	LastLogin         *time.Time `json:"last_login,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
}
