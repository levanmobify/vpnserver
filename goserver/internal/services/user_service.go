package services

import (
	"fmt"
	"math/rand"
	"os/exec"

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

	hashedPassword, err := GenerateMD5CryptHash(pass)
	if err != nil {
		return err
	}

	user.Password = pass
	user.PasswordHashed = hashedPassword

	return nil
}

func GenerateMD5CryptHash(password string) (string, error) {
	cmd := exec.Command("openssl", "passwd", "-1", password)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute openssl command: %v", err)
	}

	return string(output[:len(output)-1]), nil
}

func (userService *UserService) generatePassword(length int) (string, error) {
	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[rand.Intn(len(charset))]
	}
	return string(password), nil
}
