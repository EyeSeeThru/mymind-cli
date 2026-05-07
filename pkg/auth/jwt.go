package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const ConfigPath = "~/.config/mymind/config.json"

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// Credentials holds the kid and secret from the MyMind Extensions page.
type Credentials struct {
	Kid    string `json:"kid"`
	Secret string `json:"secret"`
}

// Load reads credentials from the config file or environment variables.
// Environment variables MYMIND_KID and MYMIND_SECRET take precedence.
// Returns nil, nil if no credentials are found.
func Load() (*Credentials, error) {
	kid := os.Getenv("MYMIND_KID")
	secret := os.Getenv("MYMIND_SECRET")
	if kid != "" && secret != "" {
		return &Credentials{Kid: kid, Secret: secret}, nil
	}
	if kid != "" || secret != "" {
		return nil, fmt.Errorf("set both MYMIND_KID and MYMIND_SECRET, or unset both to use the config file")
	}

	path := expandPath(ConfigPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no credentials file — caller can handle
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON; re-run 'mymind auth login'", path)
	}
	if creds.Kid == "" || creds.Secret == "" {
		return nil, fmt.Errorf("%s is missing kid or secret; re-run 'mymind auth login'", path)
	}
	return &creds, nil
}

// Save writes credentials to the config file with mode 0600.
func Save(creds *Credentials) error {
	dir := filepath.Dir(expandPath(ConfigPath))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	// Best-effort tighten existing dir perms
	os.Chmod(dir, 0700)

	path := expandPath(ConfigPath)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	if err := json.NewEncoder(f).Encode(creds); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing config: %w", err)
	}
	f.Close()
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming config: %w", err)
	}
	os.Chmod(path, 0600)
	return nil
}

// Clear removes the credentials file.
func Clear() error {
	path := expandPath(ConfigPath)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// SignRequest creates a fresh HS256 JWT bound to the request path and method.
// exp = iat + 300 seconds. If creds is nil, returns empty string (for dry-run previews).
func SignRequest(method, path string, creds *Credentials) (string, error) {
	if creds == nil {
		return "", nil
	}
	secret := decodeSecret(creds.Secret)
	now := time.Now().Unix()

	payload := jwt.MapClaims{
		"path":   path,
		"method": strings.ToUpper(method),
		"iat":    now,
		"exp":    now + 300,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	token.Header["kid"] = creds.Kid

	return token.SignedString(secret)
}

func decodeSecret(secretB64 string) []byte {
	// Accept base64 and base64url; strip whitespace, - → +, _ → /
	cleaned := strings.ReplaceAll(strings.ReplaceAll(secretB64, "-", "+"), "_", "/")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	// Pad to multiple of 4
	if m := len(cleaned) % 4; m != 0 {
		cleaned += strings.Repeat("=", 4-m)
	}
	b, _ := base64.StdEncoding.DecodeString(cleaned)
	if len(b) == 0 {
		// Fallback: try raw
		b, _ = base64.RawStdEncoding.DecodeString(cleaned)
	}
	if len(b) == 0 {
		// Fallback: try raw URL
		b, _ = base64.RawURLEncoding.DecodeString(cleaned)
	}
	return b
}
