package encryptor

import "golang.org/x/crypto/bcrypt"

func HashingPassword(password string) (string, error) {
	hashedByte, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", err
	}
	return string(hashedByte), nil
}

func ComparePassword(password string, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
