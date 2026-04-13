package controllers

import (
	"errors"
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func Login(c *gin.Context) {
	var user_input struct {
		Username string `json:"email_or_username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&user_input); err != nil {
		helpers.ErrorResponse(c, 400, "Format JSON tidak valid", err)
		return
	}

	var user models.User

	if err := config.DB.Where("username = ? OR email = ?", user_input.Username, user_input.Username).
		First(&user).Error; err != nil {
		
		if errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 401, "email/password does not match", nil)
		}else{
			helpers.ErrorResponse(c, 500, "Internal server error", err)
		}
		return
	}

	// cek password hash
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(user_input.Password)) != nil {
		helpers.ErrorResponse(c, 401, "email/password does not match", nil)
		return
	}

	var jwtSecret []byte = []byte(os.Getenv("JWT_SECRET"))
	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"role":  user.Role,
		"username": user.Username,
		"exp":      time.Now().AddDate(0, 1, 0).Unix(), // kadaluarsa 1 bulan
		// "exp":      time.Now().Add(time.Hour * 12).Unix(), // kadaluarsa 12 jam
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Could not create token", err)
		return
	}

	// simpan tokens
	userToken := models.UserToken{
		UserID:     user.ID,
		Token:      tokenString,
		UserAgent:  c.GetHeader("User-Agent"),
		LastUsedAt: time.Now(),
	}

	config.DB.Create(&userToken)

	c.JSON(http.StatusOK, response.Success("Login sukses!", gin.H{"token": tokenString, "user": user}))
}
func CheckToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		helpers.ErrorResponse(c, 401, "token tidak ditemukan", nil)
		c.Abort()
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		helpers.ErrorResponse(c, 401, "format token salah", nil)
		c.Abort()
		return
	}

	jwtSecret := []byte(os.Getenv("JWT_SECRET"))

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		config.DB.Where("token = ?", tokenString).Delete(&models.UserToken{})
		helpers.ErrorResponse(c, http.StatusUnauthorized, "token tidak valid atau kadaluarsa", err)
		c.Abort()
		return
	}

	// Cek token di DB
	var userToken models.UserToken
	err = config.DB.Where("token = ?", tokenString).First(&userToken).Error
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "token tidak ditemukan di database", err)
		c.Abort()
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "token tidak valid", nil)
		c.Abort()
		return
	}

	go func() {
		config.DB.Model(&userToken).Update("last_used_at", time.Now())
	}()

	c.JSON(http.StatusOK, response.Success("Token Valid", gin.H{
		"role":     claims["role"],
		"username": claims["username"],
	}))
}
func Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "token tidak ditemukan", nil)
		c.Abort()
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "format token salah", nil)
		c.Abort()
		return
	}

	// hapus token jika masih valid
	err := config.DB.Where("token = ?", tokenString).Delete(&models.UserToken{}).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Gagal menghapus token", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("Berhasil logout. Token telah dihapus", nil))
}