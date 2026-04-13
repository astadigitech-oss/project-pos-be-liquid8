package middleware

import (
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"

	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid"})
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

		go func() {
			config.DB.Model(&userToken).Update("last_used_at", time.Now())
		}()

		var user models.User
		if err := config.DB.Preload("Store").First(&user, claims["user_id"]).Error; err != nil {
			c.AbortWithStatusJSON(403, gin.H{
				"success": false,
				"message": "user not found",
			})
			return
		}

		c.Set("auth_user", user)
		c.Next()
	}
}

func RoleCheck(allowedRoles []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Ambil role user dari Context
        user := c.MustGet("auth_user").(models.User)

        isAllowed := false
        for _, role := range allowedRoles {
            if user.Role == role {
                isAllowed = true
                break
            }
        }

        if !isAllowed {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "success": false, 
                "message": "Forbidden. Insufficient permissions for this resource.",
            })
            return
        }
		
        c.Next()
    }
}

func ShiftCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// auth_user must be set by AuthCheck
		u, ok := c.Get("auth_user")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthenticated"})
			return
		}

		user := u.(models.User)
		if user.StoreID == nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "User tidak memiliki store_id"})
			return
		}

		var shift models.Shift
		if err := config.DB.Where("store_id = ? AND status = ?", *user.StoreID, "open").First(&shift).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "No active shift found"})
			return
		}

		// set shift_active in context
		c.Set("shift_active", shift)
		c.Next()
	}
}