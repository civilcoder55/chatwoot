package tenant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.yaml")
	os.WriteFile(path, []byte(`tenants:
  - phone_number: "+1234567890"
    key: "secret-1"
  - phone_number: "+0987654321"
    key: "secret-2"
`), 0o644)

	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}

	if store.FindByPhone("+1234567890") == nil {
		t.Error("expected to find tenant by phone +1234567890")
	}
	if store.FindByPhone("+0987654321") == nil {
		t.Error("expected to find tenant by phone +0987654321")
	}
	if store.FindByPhone("+9999999999") != nil {
		t.Error("expected nil for unknown phone number")
	}
}

func TestLoadStoreMissingFile(t *testing.T) {
	store, err := LoadStore("/nonexistent/path/tenants.yaml")
	if err != nil {
		t.Fatalf("LoadStore() should not error for missing file: %v", err)
	}
	if store.FindByPhone("+1234567890") != nil {
		t.Error("expected nil for empty store")
	}
}

func TestAuthenticate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.yaml")
	os.WriteFile(path, []byte(`tenants:
  - phone_number: "+1234567890"
    key: "correct-key"
`), 0o644)

	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}

	tests := []struct {
		name   string
		phone  string
		secret string
		valid  bool
	}{
		{"correct credentials", "+1234567890", "correct-key", true},
		{"wrong key", "+1234567890", "wrong-key", false},
		{"unknown phone", "+9999999999", "correct-key", false},
		{"empty phone", "", "correct-key", false},
		{"empty key", "+1234567890", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.Authenticate(tt.phone, tt.secret)
			if tt.valid && result == nil {
				t.Error("expected valid authentication, got nil")
			}
			if !tt.valid && result != nil {
				t.Error("expected nil for invalid authentication")
			}
		})
	}
}
