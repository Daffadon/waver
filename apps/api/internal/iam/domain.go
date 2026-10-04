package iam

// Admin is the single local user.
type Admin struct {
	PasswordHash string
	MustChange   bool
}

// APIKey is stored hashed. Prefix allows lookup.
type APIKey struct {
	ID        string `json:"id"`
	Prefix    string `json:"prefix"`
	Name      string `json:"name"`
	RevokedAt *int64 `json:"revoked_at,omitempty"`
	CreatedAt int64  `json:"created_at"`
}
