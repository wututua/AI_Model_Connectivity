package config

import "fmt"

func ValidateToken(token string) error {
	if len(token) < 16 || len(token) > 256 {
		return fmt.Errorf("token must be between 16 and 256 ASCII characters")
	}
	for _, character := range token {
		if character < 33 || character > 126 {
			return fmt.Errorf("token must contain only printable ASCII characters without spaces")
		}
	}
	return nil
}
