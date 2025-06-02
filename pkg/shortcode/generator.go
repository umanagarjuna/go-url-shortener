package shortcode

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultLength = 7
	charset       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// Generator generates unique short codes
type Generator interface {
	Generate() (string, error)
	GenerateWithLength(length int) (string, error)
}

// RandomGenerator generates random short codes
type RandomGenerator struct{}

func NewRandomGenerator() Generator {
	return &RandomGenerator{}
}

func (g *RandomGenerator) Generate() (string, error) {
	return g.GenerateWithLength(DefaultLength)
}

func (g *RandomGenerator) GenerateWithLength(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	for i := 0; i < length; i++ {
		b[i] = charset[b[i]%byte(len(charset))]
	}

	return string(b), nil
}

// SnowflakeGenerator implements Twitter Snowflake-like ID generation
type SnowflakeGenerator struct {
	mu        sync.Mutex
	epoch     int64
	machineID int64
	sequence  int64
	lastTime  int64
}

func NewSnowflakeGenerator(machineID int64) Generator {
	return &SnowflakeGenerator{
		epoch:     1640995200000, // Jan 1, 2022
		machineID: machineID,
	}
}

func (g *SnowflakeGenerator) Generate() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixNano() / 1e6

	if now == g.lastTime {
		g.sequence = (g.sequence + 1) & 0xFFF
		if g.sequence == 0 {
			for now <= g.lastTime {
				now = time.Now().UnixNano() / 1e6
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	id := ((now - g.epoch) << 22) |
		(g.machineID << 12) |
		g.sequence

	// Convert to base64-like string
	encoded := base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%d", id)))

	if len(encoded) > DefaultLength {
		encoded = encoded[:DefaultLength]
	}

	return encoded, nil
}

func (g *SnowflakeGenerator) GenerateWithLength(length int) (string, error) {
	code, err := g.Generate()
	if err != nil {
		return "", err
	}

	if len(code) > length {
		return code[:length], nil
	}

	return code, nil
}
