package shortcode

import (
	"crypto/rand"
	"math/big"
	"strings"
)

type Generator interface {
	Generate() (string, error)
	GenerateWithLength(length int) (string, error)
}

type Base62Generator struct {
	length  int
	charset string
}

const (
	// Base62 charset (0-9, A-Z, a-z)
	DefaultCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	DefaultLength  = 8 // Increased from 6 to 8 for more unique combinations
)

func NewBase62Generator() *Base62Generator {
	return &Base62Generator{
		length:  DefaultLength,
		charset: DefaultCharset,
	}
}

func NewBase62GeneratorWithLength(length int) *Base62Generator {
	if length < 4 {
		length = 4 // Minimum length for security
	}
	if length > 12 {
		length = 12 // Maximum length for practicality
	}

	return &Base62Generator{
		length:  length,
		charset: DefaultCharset,
	}
}

func (g *Base62Generator) Generate() (string, error) {
	return g.GenerateWithLength(g.length)
}

func (g *Base62Generator) GenerateWithLength(length int) (string, error) {
	if length <= 0 {
		length = g.length
	}

	var result strings.Builder
	result.Grow(length)

	charsetLength := big.NewInt(int64(len(g.charset)))

	for i := 0; i < length; i++ {
		// Use crypto/rand for better randomness
		randomIndex, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", err
		}

		result.WriteByte(g.charset[randomIndex.Int64()])
	}

	return result.String(), nil
}

// Alternative: UUID-based generator for even better uniqueness
type UUIDGenerator struct {
	length int
}

func NewUUIDGenerator(length int) *UUIDGenerator {
	if length < 6 {
		length = 6
	}
	if length > 12 {
		length = 12
	}

	return &UUIDGenerator{length: length}
}

func (g *UUIDGenerator) Generate() (string, error) {
	return g.GenerateWithLength(g.length)
}

func (g *UUIDGenerator) GenerateWithLength(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Convert to base62
	charset := DefaultCharset
	var result strings.Builder
	result.Grow(length)

	for _, b := range bytes {
		result.WriteByte(charset[int(b)%len(charset)])
	}

	return result.String(), nil
}

// Timestamp-based generator for chronological ordering (optional)
type TimestampGenerator struct {
	length int
	base62 *Base62Generator
}

func NewTimestampGenerator(length int) *TimestampGenerator {
	return &TimestampGenerator{
		length: length,
		base62: NewBase62GeneratorWithLength(length),
	}
}

func (g *TimestampGenerator) Generate() (string, error) {
	return g.GenerateWithLength(g.length)
}

func (g *TimestampGenerator) GenerateWithLength(length int) (string, error) {
	// For now, just use the base62 generator
	// You could implement timestamp encoding here if needed
	return g.base62.GenerateWithLength(length)
}
