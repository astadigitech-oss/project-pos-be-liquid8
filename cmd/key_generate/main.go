package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const envFile = ".env"

const (
	JWTSecretKeyVar = "JWT_SECRET_KEY"
	AppKeyVar       = "APP_KEY"
)

func main() {
	jwtSecret := InitEnvKey(JWTSecretKeyVar, 32, "hex")
	appKey := InitEnvKey(AppKeyVar, 32, "base64")

	fmt.Println("✅ JWT_SECRET_KEY:", jwtSecret)
	fmt.Println("✅ APP_KEY:", appKey)
}

// mode = "hex" atau "base64"
func InitEnvKey(keyName string, length int, mode string) string {
	content, err := os.ReadFile(envFile)
	if err != nil {
		fmt.Println(".env belum ada, membuat baru...")
		content = []byte("")
	}

	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, keyName+"=") {
			existingKey := strings.TrimPrefix(line, keyName+"=")
			existingKey = strings.TrimSpace(existingKey)
			fmt.Printf("✅ %s sudah ada di .env\n", keyName)
			return existingKey
		}
	}

	// Generate key baru
	newKey := generateSecretKey(length, mode)
	newLine := fmt.Sprintf("%s=%s\n", keyName, newKey)

	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err := f.WriteString(newLine); err != nil {
		panic(err)
	}

	fmt.Printf("🔐 %s berhasil dibuat dan disimpan di .env\n", keyName)

	return newKey
}

func generateSecretKey(length int, mode string) string {
	key := make([]byte, length)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}

	switch mode {
	case "hex":
		return hex.EncodeToString(key)
	case "base64":
		return base64.StdEncoding.EncodeToString(key)
	default:
		panic("mode harus hex atau base64")
	}
}