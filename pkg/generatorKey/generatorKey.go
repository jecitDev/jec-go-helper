package generatorKey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// GenerateRandomSeed generates a cryptographically secure random seed for the license key
func generateRandomSeed(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random seed: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// GenerateLicenseKey generates a license key using a random seed and SHA-256 hashing
func generateLicenseKey(seed string) string {
	hash := sha256.New()
	hash.Write([]byte(seed))
	hashedBytes := hash.Sum(nil)
	encodedString := base64.StdEncoding.EncodeToString(hashedBytes)

	// Replace characters that are not suitable for keys
	encodedString = strings.NewReplacer("/", "", "+", "", "=", "").Replace(encodedString)

	return encodedString
}

// CreateLicenseKey generates and formats a license key
func CreateLicenseKey(seedLength, keySegmentLength int) (string, error) {
	seed, err := generateRandomSeed(seedLength)
	if err != nil {
		return "", err
	}
	licenseKey := generateLicenseKey(seed)

	// Format the key to include dashes
	var formattedKey strings.Builder
	for i := 0; i < len(licenseKey); i += keySegmentLength {
		if i > 0 {
			formattedKey.WriteString("-")
		}
		end := i + keySegmentLength
		if end > len(licenseKey) {
			end = len(licenseKey)
		}
		formattedKey.WriteString(licenseKey[i:end])
	}

	return formattedKey.String(), nil
}
