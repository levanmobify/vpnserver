package services

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"math/big"

	"github.com/LevanPro/server/internal/models"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%"
const passwordLength = 20

type UserService struct {
}

func NewUserService() *UserService {
	return &UserService{}
}

func (userService *UserService) AddPassword(user *models.User) error {
	pass, err := userService.generatePassword(passwordLength)
	if err != nil {
		return err
	}

	hashedPassword := userService.generatePashedPassowrd(pass)

	user.Password = pass
	user.PasswordHashed = hashedPassword

	return nil
}

func (userService *UserService) generatePashedPassowrd(password string) string {
	hash := md5.Sum([]byte(password))
	return hex.EncodeToString(hash[:])
}

func (userService *UserService) generatePassword(length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		randomByte, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[randomByte.Int64()]
	}
	return string(b), nil
}
