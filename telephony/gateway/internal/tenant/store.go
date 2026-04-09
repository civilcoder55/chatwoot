// Package tenant provides a simple YAML-backed tenant registry.
package tenant

import (
	"fmt"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Tenant represents a single tenant entry.
type Tenant struct {
	PhoneNumber string `yaml:"phone_number"`
	Key         string `yaml:"key"`
}

// file schema
type tenantsFile struct {
	Tenants []Tenant `yaml:"tenants"`
}

// Store holds the loaded tenant list keyed by phone number.
type Store struct {
	mu      sync.RWMutex
	byPhone map[string]*Tenant
}

// LoadStore reads tenants from a YAML file and returns a Store.
// If the file does not exist, an empty store is returned (no error).
func LoadStore(path string) (*Store, error) {
	s := &Store{byPhone: make(map[string]*Tenant)}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Str("path", path).Msg("tenants file not found, running without tenants")
			return s, nil
		}
		return nil, fmt.Errorf("read tenants file: %w", err)
	}

	var f tenantsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse tenants file: %w", err)
	}

	for i := range f.Tenants {
		t := &f.Tenants[i]
		s.byPhone[t.PhoneNumber] = t
	}

	log.Info().Int("count", len(s.byPhone)).Msg("tenants loaded")
	return s, nil
}

// FindByPhone returns the tenant for the given phone number, or nil.
func (s *Store) FindByPhone(phone string) *Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byPhone[phone]
}

// Authenticate checks that the phone number exists and the api key matches.
func (s *Store) Authenticate(phone, secret string) *Tenant {
	t := s.FindByPhone(phone)
	if t != nil && t.Key == secret {
		return t
	}
	return nil
}
