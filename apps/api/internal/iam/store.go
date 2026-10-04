package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Store is the infrastructure layer for sqlite.
type Store struct{ DB *sql.DB }

func (s *Store) GetAdmin() (Admin, bool, error) {
	var h string
	var mc int
	var updated int64
	err := s.DB.QueryRow(`SELECT password_hash, must_change, updated_at FROM admin WHERE id=1`).Scan(&h, &mc, &updated)
	if err == sql.ErrNoRows {
		return Admin{}, false, nil
	}
	if err != nil {
		return Admin{}, false, err
	}
	return Admin{PasswordHash: h, MustChange: mc == 1}, true, nil
}

func (s *Store) PutAdmin(hash string, mustChange bool) error {
	now := time.Now().UTC().Unix()
	_, err := s.DB.Exec(`INSERT INTO admin(id,password_hash,must_change,updated_at) VALUES(1,?,?,?)
		ON CONFLICT(id) DO UPDATE SET password_hash=excluded.password_hash, must_change=excluded.must_change, updated_at=excluded.updated_at`,
		hash, boolToInt(mustChange), now)
	return err
}

func (s *Store) InsertKey(k APIKey, hash string) error {
	_, err := s.DB.Exec(`INSERT INTO api_keys(id,prefix,hash,name,revoked_at,created_at) VALUES(?,?,?,?,?,?)`,
		k.ID, k.Prefix, hash, k.Name, k.RevokedAt, k.CreatedAt)
	return err
}

func (s *Store) ListKeys() ([]APIKey, error) {
	rows, err := s.DB.Query(`SELECT id,prefix,name,revoked_at,created_at FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var rev sql.NullInt64
		if err := rows.Scan(&k.ID, &k.Prefix, &k.Name, &rev, &k.CreatedAt); err != nil {
			return nil, err
		}
		if rev.Valid {
			v := rev.Int64
			k.RevokedAt = &v
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) RevokeKey(id string) error {
	now := time.Now().UTC().Unix()
	_, err := s.DB.Exec(`UPDATE api_keys SET revoked_at=? WHERE id=?`, now, id)
	return err
}

// ValidKey checks Bearer key against sha256 hashes. Revoked keys fail.
func (s *Store) ValidKey(plain string) bool {
	h := sha256.Sum256([]byte(plain))
	want := base64.RawURLEncoding.EncodeToString(h[:])
	rows, err := s.DB.Query(`SELECT hash FROM api_keys WHERE revoked_at IS NULL`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var got string
		if err := rows.Scan(&got); err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			return true
		}
	}
	return false
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// RandomPassword returns 16-char url-safe secret for first boot.
func RandomPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RandomKey returns wv_<id> + secret. Only hash is stored.
func RandomKey() (id, prefix, plain string, err error) {
	ib := make([]byte, 6)
	sb := make([]byte, 24)
	if _, err = rand.Read(ib); err != nil {
		return
	}
	if _, err = rand.Read(sb); err != nil {
		return
	}
	id = base64.RawURLEncoding.EncodeToString(ib)
	prefix = "wv_" + id[:6]
	plain = prefix + "." + base64.RawURLEncoding.EncodeToString(sb)
	return id, prefix, plain, nil
}

func KeyHash(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func MintJWT(secret, sub string) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub,
		"exp": time.Now().UTC().Add(12 * time.Hour).Unix(),
	})
	return t.SignedString([]byte(secret))
}

func ParseJWT(secret, tok string) bool {
	_, err := jwt.Parse(tok, func(t *jwt.Token) (any, error) { return []byte(secret), nil })
	return err == nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
