package validator

import (
	"fmt"
	"net/url"
	"strings"
)

// URLValidator validates URLs
type URLValidator interface {
	Validate(rawURL string) error
	IsSafe(rawURL string) (bool, error)
}

type DefaultValidator struct {
	blacklistedDomains []string
	maxRedirects       int
}

func NewDefaultValidator() URLValidator {
	return &DefaultValidator{
		blacklistedDomains: []string{
			"bit.ly", "tinyurl.com", // Prevent recursive shortening
		},
		maxRedirects: 5,
	}
}

func (v *DefaultValidator) Validate(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only HTTP(S) URLs are allowed")
	}

	// Check host
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Check blacklist
	for _, domain := range v.blacklistedDomains {
		if strings.Contains(u.Host, domain) {
			return fmt.Errorf("domain %s is blacklisted", domain)
		}
	}

	return nil
}

func (v *DefaultValidator) IsSafe(rawURL string) (bool, error) {
	// In production, integrate with Google Safe Browsing API
	// or similar service
	return true, nil
}
