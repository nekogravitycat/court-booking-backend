package auth

import "golang.org/x/crypto/bcrypt"

// PasswordHasher defines behavior for hashing and comparing passwords.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Compare(hash, plain string) error
}

// BcryptPasswordHasher is a PasswordHasher implementation using bcrypt.
type BcryptPasswordHasher struct {
	cost int
}

// NewBcryptPasswordHasher creates a new BcryptPasswordHasher with default cost.
func NewBcryptPasswordHasher() *BcryptPasswordHasher {
	return &BcryptPasswordHasher{
		cost: bcrypt.DefaultCost,
	}
}

// NewBcryptPasswordHasherWithCost allows you to specify a custom bcrypt cost.
func NewBcryptPasswordHasherWithCost(cost int) *BcryptPasswordHasher {
	return &BcryptPasswordHasher{
		cost: cost,
	}
}

// Hash hashes the given plain password string using bcrypt.
func (h *BcryptPasswordHasher) Hash(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Compare compares a bcrypt hashed password with its possible plaintext equivalent.
// Returns nil on success, or an error on failure.
func (h *BcryptPasswordHasher) Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
