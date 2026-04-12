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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "data tidak valid",
			"msg":   err.Error(),
		})
		return
	}

	var user models.User

	if err := config.DB.Where("username = ? OR email = ?", user_input.Username, user_input.Username).
		First(&user).Error; err != nil {
		
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, response.Error("email/password does not match"))
		}else{
			helpers.ErrorResponse(c, 500, "Internal server error", err)
		}
		return
	}

	// cek password hash
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(user_input.Password)) != nil {
		c.JSON(http.StatusUnauthorized, response.Error("email/password does not match"))
		return
	}

	var jwtSecret []byte = []byte(os.Getenv("JWT_SECRET"))
	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"role":  user.Role,
		"username": user.Username,
		// "exp":      time.Now().AddDate(0, 1, 0).Unix(), // kadaluarsa 1 bulan
		"exp":      time.Now().Add(time.Hour * 12).Unix(), // kadaluarsa 12 jam
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error("Could not create token"))
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak ditemukan"})
		c.Abort()
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "format token salah"})
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid atau kadaluarsa"})
		c.Abort()
		return
	}

	// Cek token di DB
	var userToken models.UserToken
	err = config.DB.Where("token = ?", tokenString).First(&userToken).Error
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak ditemukan di database"})
		c.Abort()
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid"})
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
