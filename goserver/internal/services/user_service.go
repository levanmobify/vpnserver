package services

import (
	"crypto/md5"
	"encoding/hex"
	"math/rand"

	"github.com/LevanPro/server/internal/models"
)

const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnopqrstuvwxyz23456789"
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
	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[rand.Intn(len(charset))]
	}
	return string(password), nil
}
