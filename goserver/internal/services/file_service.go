package services

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/LevanPro/server/internal/models"
)

type FileService struct {
	storagePath string
}

func NewFileService(folderPath string) *FileService {
	return &FileService{
		storagePath: folderPath,
	}
}

func (fileService *FileService) ReadFile() ([]models.User, error) {
	sourceFile := filepath.Join(fileService.storagePath, "/ppp/chap-secrets")

	result := make([]models.User, 0)

	maxRetries := 3

	var file *os.File
	var err error

	for i := 0; i < maxRetries; i++ {
		file, err = os.OpenFile(sourceFile, os.O_RDONLY, 0)
		if err != nil {
			if i == maxRetries-1 {
				return result, fmt.Errorf("error opening file after %d retries: %v", maxRetries, err)
			}

			time.Sleep(100 * time.Millisecond)
			continue
		}

		err = syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err != nil {
			file.Close()
			if i == maxRetries-1 {
				return result, err
			}

			time.Sleep(100 * time.Millisecond)
			continue
		}

		break
	}

	defer func() {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
	}()

	psk, err := fileService.ReadPSKSecret()

	if err != nil {
		return result, err
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) > 0 {
			username := fields[0]
			username = strings.Trim(username, "\"")

			result = append(result, models.User{
				Username:  username,
				PSKSecret: psk,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}

func (fileService *FileService) AddUsers(users []models.User) error {
	if err := fileService.appendToChapSecrets(users); err != nil {
		return fmt.Errorf("failed to update chap-secrets: %w", err)
	}

	if err := fileService.appendToIpsecPasswd(users); err != nil {
		return fmt.Errorf("failed to update ipsec passwd: %w", err)
	}

	return nil
}

func (fileService *FileService) ReadPSKSecret() (string, error) {
	path := filepath.Join(fileService.storagePath, "/ipsec.secrets")

	file, err := os.Open(path) // Open without exclusive lock
	if err != nil {
		return "", err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	re := regexp.MustCompile(`PSK\s+"([^"]+)"`)

	var pskValue string
	for scanner.Scan() {
		line := scanner.Text()
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			pskValue = match[1]
			break // Stop at the first match
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if pskValue != "" {
		return pskValue, nil
	}

	return "", errors.New("no psk found")
}

func (fileService *FileService) appendToChapSecrets(users []models.User) error {
	path := filepath.Join(fileService.storagePath, "/ppp/chap-secrets")
	return appendToFile(path, users, "chap-secrets")
}

func (fileService *FileService) appendToIpsecPasswd(users []models.User) error {
	path := filepath.Join(fileService.storagePath, "/ipsec.d/passwd")
	return appendToFile(path, users, "passwd")
}

func appendToFile(filePath string, users []models.User, contentType string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock file %s: %w", filePath, err)
	}

	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	for _, user := range users {
		content := fmt.Sprintf("%s:%s:xauth-psk\n", user.Username, user.PasswordHashed)

		if contentType == "chap-secrets" {
			content = fmt.Sprintf("\"%s\" l2tpd \"%s\" *\n", user.Username, user.Password)
		}

		if _, err := file.WriteString(content); err != nil {
			return fmt.Errorf("failed to write to file %s: %w", filePath, err)
		}
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file %s: %w", filePath, err)
	}

	return nil
}
