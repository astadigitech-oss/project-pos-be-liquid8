package controllers

import (
	"liquid8/pos/config"
	"liquid8/pos/helpers"

	// "liquid8/pos/helpers"
	"liquid8/pos/models"

	// "errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// "gorm.io/gorm"
	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"
)

func GetUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	var users []models.User
	var totalData int64

	//inisialisasi query
	query := config.DB.Model(&models.User{}).Where("role NOT IN ?", []string{"superadmin", "admin"})

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		query = query.Where("(name LIKE ? OR email LIKE ?)", searchPattern, searchPattern)
	}

	// Menghitung total data yang sesuai dengan filter/search sebelum diterapkan limit/offset
	if err := query.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghitung total data pengguna", "error": err.Error()})
		return
	}

	err := query.
		Limit(limit).
		Offset(offset).
		Order("created_at desc"). // Sorting data terbaru di atas
		Find(&users).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}


	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	// pagination links
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(users), int(totalData))

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Users",
			"resource": gin.H{
				"data": users,
				"pagination": pagination,
			},
		},
	})
}
func DetailUser(c *gin.Context) {
	userID := c.Param("id")

	var user models.User
	if err := config.DB.Preload("Role").First(&user, userID).Error; err != nil {
		helpers.ErrorResponse(c, http.StatusNotFound, "User tidak ditemukan", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Detail User",
			"resource": user,
		},
	})
}
func CreateUser(c *gin.Context) {
	type CreateUserRequest struct {
		StoreID  uint   `json:"store_id" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Username     string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
		Role     string `json:"role" binding:"required,oneof=admin kasir"`
	}

	var req CreateUserRequest

	// VALIDASI REQUEST
	// =========================
	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "storeid":
				if e.Tag() == "required" {
					errors["store_id"] = "Store ID wajib diisi"
				}
			case "name":
				if e.Tag() == "required" {
					errors["name"] = "Nama wajib diisi"
				}

			case "email":
				if e.Tag() == "required" {
					errors["email"] = "Email wajib diisi"
				}
				if e.Tag() == "email" {
					errors["email"] = "Format email tidak valid"
				}

			case "password":
				if e.Tag() == "required" {
					errors["password"] = "Password wajib diisi"
				}
				if e.Tag() == "min" {
					errors["password"] = "Password minimal 6 karakter"
				}

			case "role":
				if e.Tag() == "required" {
					errors["role"] = "Role wajib dipilih"
				}else {
					errors["role"] = "Role harus salah satu dari admin, kasir"
				}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errors,
		})
		
		return
	}

	var store models.StoreProfile
	if err := config.DB.First(&store, req.StoreID).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Store ID tidak ditemukan", err)
		return
	}	

	// CEK EMAIL SUDAH ADA
	// =========================
	var count int64
	if err := config.DB.
		Model(&models.User{}).
		Where("email = ?", req.Email).
		Count(&count).Error; err != nil {

		helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal memeriksa email", err)
		return
	}

	if count > 0 {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Email sudah terdaftar", nil)
		return
	}

	// HASH PASSWORD
	// =========================
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal generate password", err)
		return
	}

	// CREATE USER
	// =========================
	storeID := uint64(store.ID)
	user := models.User{
		StoreID:  &storeID,
		Name:     req.Name,
		Username:     req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		Role:     req.Role,
	}

	if err := config.DB.Create(&user).Error; err != nil {
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal membuat user", err)
		return
	}

	// RESPONSE
	// =========================
	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "User berhasil dibuat",
			"resource": user,
		},
	})
}
func UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	type payload struct {
		StoreID  uint   `json:"store_id" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"omitempty,min=6"`
		Role     string `json:"role" binding:"required,oneof=admin kasir"`
	}

	var req payload

	// =========================
	// VALIDASI REQUEST
	// =========================
	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Payload tidak valid",
			})
			return
		}

		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "storeid":
				if e.Tag() == "required" {
					errors["store_id"] = "Store ID wajib diisi"
				}
			case "name":
				if e.Tag() == "required" {
					errors["name"] = "Nama wajib diisi"
				}
			case "username":
				if e.Tag() == "required" {
					errors["username"] = "Username wajib diisi"
				}
			case "email":
				if e.Tag() == "email" {
					errors["email"] = "Format email tidak valid"
				} else {
					errors["email"] = "Email wajib diisi"
				}
			case "password":
				if e.Tag() == "min" {
					errors["password"] = "Password minimal 6 karakter"
				}
			case "role":
				if e.Tag() == "required" {
					errors["role"] = "Role wajib dipilih"
				}else {
					errors["role"] = "Role harus salah satu dari admin, kasir"
				}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	var store models.StoreProfile
	if err := config.DB.First(&store, req.StoreID).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Store ID tidak ditemukan", err)
		return
	}	

	// =========================
	// AMBIL USER EXISTING
	// =========================
	var user models.User
	if err := config.DB.Where("role = ?", "kasir").First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "User tidak ditemukan",
		})
		return
	}

	// =========================
	// CEK EMAIL UNIK (KECUALI DIRI SENDIRI)
	// =========================
	var count int64
	if err := config.DB.
		Model(&models.User{}).
		Where("email = ? AND id != ?", req.Email, userID).
		Count(&count).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Email sudah digunakan",
		})
		return
	}

	// =========================
	// UPDATE DATA USER
	// =========================
	user.Name = req.Name
	user.Username = req.Username
	user.Email = req.Email
	user.Role = req.Role

	// Password hanya diupdate kalau diisi
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword(
			[]byte(req.Password),
			bcrypt.DefaultCost,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		user.Password = string(hashedPassword)
	}

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal mengupdate user",
			"error":   err.Error(),
		})
		return
	}

	// =========================
	// RESPONSE
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User berhasil diupdate",
		"data":    user,
	})
}
func UpdateProfile(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	var req struct {
		Name     string `json:"name" binding:"required"`
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Payload tidak valid",
			})
			return
		}

		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "name":
				errors["name"] = "Nama wajib diisi"
			case "username":
				errors["username"] = "Username wajib diisi"
			case "email":
				if e.Tag() == "email" {
					errors["email"] = "Format email tidak valid"
				} else {
					errors["email"] = "Email wajib diisi"
				}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}	

	var userOther models.User
	if err := config.DB.
		Model(&models.User{}).
		Where("email = ? AND username = ? AND id != ?", req.Email, req.Username, user.ID).
		First(&userOther).Error; err != nil {

		helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal memeriksa email", err)
		return
	}

	if userOther.Email == req.Email {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Email sudah digunakan",
		})
		return
	}else if userOther.Username == req.Username {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Username sudah digunakan",
		})
		return
	}

	user_update := map[string]interface{}{
		"name":     req.Name,
		"username": req.Username,
		"email":    req.Email,
	}

	if err := config.DB.Model(&user).Updates(user_update).Error; err != nil {
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User profile berhasil diupdate",
		"resource":    user,
	})
}
func ChangePassword(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Payload tidak valid",
			})
			return
		}

		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "oldpassword":
				errors["old_password"] = "Old Password wajib diisi"
			case "newpassword":
				if e.Tag() == "required" {
					errors["new_password"] = "New Password wajib diisi"
				} else {
					errors["new_password"] = "New Password minimal 6 karakter"
				}
			default:
				errors[field] = "Field tidak valid"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}	

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)) != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Old password salah", nil)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal generate password", err)
		return
	}

	new_pass := string(hashedPassword)
	if err := config.DB.Model(&user).Update("password", new_pass).Error; err != nil {
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Failed to update user password", err)
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User password berhasil diupdate",
	})
}
func DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// =========================
	// AMBIL USER
	// =========================
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "User tidak ditemukan",
		})
		return
	}

	// =========================
	// DELETE USER
	// =========================
	if err := config.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menghapus user",
			"error":   err.Error(),
		})
		return
	}

	// =========================
	// RESPONSE
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User berhasil dihapus",
	})
}
func UserInfo(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	user_info := gin.H{
		"id":       user.ID,
		"name":     user.Name,
		"email":    user.Email,
		"username": user.Username,
		"role":     user.Role,
		"store_name": nil,
	}

	if user.StoreID != nil {
		var userstore models.StoreProfile
		if err := config.DB.First(&userstore, "id = ?", *user.StoreID).Error; err != nil {
			helpers.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data toko", err)
			return
		}

		user_info["store_name"] = userstore.StoreName
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User info retrieved successfully",
		"resource":   user_info,
	})
}





