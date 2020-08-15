package server

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateSecretKeyBase() (string, error) {
	secretKey := make([]byte, 64)
	_, err := rand.Read(secretKey)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(secretKey), nil
}
