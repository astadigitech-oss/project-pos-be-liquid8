package controllers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

func ListAllMembers(c *gin.Context) {
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    if page < 1 { page = 1 }

    offset := (page-1)*limit

    baseWhere := ""
    args := []interface{}{}
    if q != "" { 
		baseWhere = "WHERE name LIKE ? OR email LIKE ?"; 
		like := "%"+q+"%"; 
		args = append(args, like, like) 
	}

    var total int64
    countSQL := fmt.Sprintf("SELECT COUNT(*) FROM members %s", baseWhere)
    if len(args) > 0 { 
		if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	} else {
		if err := config.DB.Raw(countSQL).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	}

    type Row struct {
        ID uint `json:"id"`
        Code string `json:"code"`
        Name string `json:"name"`
        Email string `json:"email"`
        Phone string `json:"phone"`
    }
    var rows []Row

    dataSQL := fmt.Sprintf(`
		SELECT 
			id, 
			code, 
			name, 
			email, 
			phone 
		FROM members %s 
		ORDER BY created_at DESC LIMIT ? OFFSET ?`, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil { helpers.ErrorResponse(c, 500, "Failed to fetch members", err); return }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    c.JSON(http.StatusOK, response.Success("Members", gin.H{"data": rows, "pagination": pagination}))
}
func DetailMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil {
		helpers.ErrorResponse(c, 400, "Invalid id", err);
		return
	}

    var m models.Member
    if err := config.DB.First(&m, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Member not found", err);
		return
	}
    c.JSON(http.StatusOK, response.Success("Detail Member", m))
}
func CreateMember(c *gin.Context) {
	type payload struct {
        Name string  `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
		Phone string `json:"phone" binding:"required"`
		NoKtp string `json:"no_ktp" binding:"required"`
    }

    var p payload
    if err := c.ShouldBindJSON(&p); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}
		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "Name":
				if e.Tag() == "required" {
					errorsMap["name"] = "Nama wajib diisi"
				}
			case "Email":
				if e.Tag() == "required" {
					errorsMap["email"] = "Email wajib diisi"
				} else if e.Tag() == "email" {
					errorsMap["email"] = "Format email tidak valid"
				}
			case "Phone":
				if e.Tag() == "required" {
					errorsMap["phone"] = "Telepon wajib diisi"
				}
			case "NoKtp":
				if e.Tag() == "required" {
					errorsMap["no_ktp"] = "No KTP wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}
	
	var otherM models.Member
	if err := config.DB.Model(&models.Member{}).
		Where("email = ? OR phone = ? OR no_ktp = ?", p.Email, p.Phone, p.NoKtp).
		First(&otherM).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 500, "Failed query", err)
			return
		}
	}
	// cek field mana yang duplicate
	if otherM.Email == p.Email {
		helpers.ErrorResponse(c, 422, "Email already in use", nil)
		return
	}
	if otherM.Phone == p.Phone {
		helpers.ErrorResponse(c, 422, "Phone already in use", nil)
		return
	}
	if otherM.NoKtp == p.NoKtp {
		helpers.ErrorResponse(c, 422, "KTP already in use", nil)
		return
	}

	code := helpers.RandomString(5)
    member := models.Member{
		Code:    code,
		Name:  p.Name,
		Email: p.Email,
		Phone: p.Phone,
		NoKtp: p.NoKtp,
	}

    if err := config.DB.Create(&member).Error; err != nil { 
		helpers.ErrorResponse(c, 500, "Failed to create member", err); 
		return 
	}

    c.JSON(http.StatusOK, response.Success("Member created", member))
}
func UpdateMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil { 
		helpers.ErrorResponse(c, 400, "Invalid id", err); 
		return 
	}

	var p struct {
        Name string  `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
		Phone string `json:"phone" binding:"required"`
		NoKtp string `json:"no_ktp" binding:"required"`
    }
    if err := c.ShouldBindJSON(&p); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}
		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "Name":
				if e.Tag() == "required" {
					errorsMap["name"] = "Nama wajib diisi"
				}
			case "Email":
				if e.Tag() == "required" {
					errorsMap["email"] = "Email wajib diisi"
				} else if e.Tag() == "email" {
					errorsMap["email"] = "Format email tidak valid"
				}
			case "Phone":
				if e.Tag() == "required" {
					errorsMap["phone"] = "Telepon wajib diisi"
				}
			case "NoKtp":
				if e.Tag() == "required" {
					errorsMap["no_ktp"] = "No KTP wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}
	
    var m models.Member
    if err := config.DB.First(&m, id).Error; err != nil { 
		helpers.ErrorResponse(c, 404, "Member not found", err); return 
	}

	var otherM models.Member
	if err := config.DB.Model(&models.Member{}).
		Where("id != ?", id).
		Where("email = ? OR phone = ? OR no_ktp = ?", p.Email, p.Phone, p.NoKtp).
		First(&otherM).Error; err != nil {
		
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 500, "Failed query", err)
			return
		}
	}
	// cek field mana yang duplicate
	if otherM.Email == p.Email {
		helpers.ErrorResponse(c, 422, "Email already in use", nil)
		return
	}
	if otherM.Phone == p.Phone {
		helpers.ErrorResponse(c, 422, "Phone already in use", nil)
		return
	}
	if otherM.NoKtp == p.NoKtp {
		helpers.ErrorResponse(c, 422, "KTP already in use", nil)
		return
	}

	if err := config.DB.Model(&m).Updates(models.Member{
		Name:  p.Name,
		Email: p.Email,
		Phone: p.Phone,
		NoKtp: p.NoKtp,
	}).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to update member", err); 
		return
	}

    c.JSON(http.StatusOK, response.Success("Member updated", m))
}
func DeleteMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil { 
		helpers.ErrorResponse(c, 400, "Invalid id", err); 
		return 
	}
	var m models.Member
	if err := config.DB.First(&m, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Member not found", err);
		return
	}
	if err := config.DB.Delete(&m).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to delete member", err);
		return
	}

	c.JSON(http.StatusOK, response.Success("Member deleted", nil))
}
