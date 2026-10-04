package auth

import (
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	for _, value := range []string{"Abc1234", "abcdefgh1", "ABCDEFGH1", "Abcdefgh", strings.Repeat("Aa1", 400)} {
		if ValidatePassword(value) == nil {
			t.Errorf("accepted invalid password %q", value)
		}
	}
	for _, value := range []string{"Abcd1234", "Abcd1234!", "Abcd1234 spaces"} {
		if err := ValidatePassword(value); err != nil {
			t.Error(err)
		}
	}
}

func TestPasswordsAreSaltedAndVerified(t *testing.T) {
	left, err := HashPassword("Abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	right, _ := HashPassword("Abcd1234")
	if left == right || strings.Contains(left, "Abcd1234") {
		t.Fatal("password was not uniquely salted")
	}
	if !VerifyPassword(left, "Abcd1234") || VerifyPassword(left, "Wrong123") || VerifyPassword("invalid", "Abcd1234") {
		t.Fatal("password verification failed")
	}
}

func TestUsernameValidation(t *testing.T) {
	for _, name := range []string{"ab", "with space", "../admin", strings.Repeat("a", 33)} {
		if ValidateUsername(name) == nil {
			t.Errorf("invalid username %q accepted", name)
		}
	}
	if NormalizeUsername(" Admin ") != "admin" || ValidateUsername("user.name-1") != nil {
		t.Fatal("valid username rejected")
	}
}
