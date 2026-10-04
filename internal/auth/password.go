package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const passwordIterations = 600000

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$`)

func NormalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func ValidateUsername(value string) error {
	if !usernamePattern.MatchString(value) {
		return errors.New("用户名须为 3-32 位字母、数字、点、下划线或短横线，并以字母或数字开头")
	}
	return nil
}

func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 1024 {
		return errors.New("密码至少 8 位，且不超过 1024 字节")
	}
	var upper, lower, digit bool
	for _, c := range password {
		upper = upper || c >= 'A' && c <= 'Z'
		lower = lower || c >= 'a' && c <= 'z'
		digit = digit || c >= '0' && c <= '9'
	}
	if !upper || !lower || !digit {
		return errors.New("密码必须包含大写字母、小写字母和数字")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$600000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) bool {
	// Unknown users still run the same KDF to avoid a cheap username oracle.
	salt := make([]byte, 16)
	expected := make([]byte, 32)
	valid := false
	parts := strings.Split(encoded, "$")
	if len(parts) == 4 && parts[0] == "pbkdf2-sha256" && parts[1] == "600000" {
		s, e1 := base64.RawStdEncoding.DecodeString(parts[2])
		k, e2 := base64.RawStdEncoding.DecodeString(parts[3])
		if e1 == nil && e2 == nil && len(s) == 16 && len(k) == 32 {
			salt, expected, valid = s, k, true
		}
	}
	if len(password) > 1024 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1 && valid
}
