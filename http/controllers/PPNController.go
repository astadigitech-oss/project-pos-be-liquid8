package controllers

import (
	"errors"
	"fmt"
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"
	"strings"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
	// "github.com/go-playground/validator/v10"
)

func GetPPN(c *gin.Context)  {
	var ppn []models.Ppn
	if err := config.DB.Find(&ppn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengambil data ppn",
			"error": err.Error(),
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Data PPN",
		"data": ppn,
	})
}
func DetailPPN(c *gin.Context) {
	ppnID := c.Param("ppn_id")

	var ppn models.Ppn
	if err := config.DB.First(&ppn, ppnID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 404, "Data ppn tidak ditemukan", nil)
		} else {
			helpers.ErrorResponse(c, 404, "Gagal mengambil dat ppn", err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Detail PPN",
		"data":    ppn,
	})
}
func StorePPN(c *gin.Context) {
	type payloadRequest struct {
		PPN float64 `json:"ppn" binding:"required,numeric"`
	}

	var payload payloadRequest

	// Bind & validate
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors":  err.Error(),
		})
		return
	}

	db := config.DB

	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	// UNIQUE CHECK
	var count int64
	db.Model(&models.Ppn{}).
		Where("ppn = ?", payload.PPN).
		Count(&count)

	if count > 0 {
		helpers.ErrorResponse(c, 422, fmt.Sprintf("PPN %.2f sudah terdaftar", payload.PPN), nil)
		return
	}

	// CREATE
	ppn := models.Ppn{
		Ppn: payload.PPN,
	}

	if err := db.Create(&ppn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal menambah data PPN",
			"error": err.Error(),
		})
		return
	}

	// SUCCESS RESPONSE
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil menambah data PPN",
		"data":    ppn,
	})
}
func UpdatePPN(c *gin.Context) {
	ppn_id := c.Param("ppn_id")

	type payloadRequest struct {
		PPN float64 `json:"ppn" binding:"required,numeric"`
		IsTaxDefault *bool `json:"is_tax_default"`
	}

	var payload payloadRequest

	// Bind & validate
	if err := c.ShouldBindJSON(&payload); err != nil {
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
				case "ppn":
					errors["ppn"] = "Nilai PPN wajib diisi"
				default: 
					errors[field] = "Format tidak valid pada"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}	

	db := config.DB

	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	//check data ppn
	var existingPPN models.Ppn
	if err := db.First(&existingPPN, ppn_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 404, "Data ppn tidak ditemukan", nil)
		} else {
			helpers.ErrorResponse(c, 404, "Gagal mengambil dat ppn", err)
		}

		return
	}

	// UNIQUE CHECK
	var count int64
	db.Model(&models.Ppn{}).
		Where("id != ?", existingPPN.ID).
		Where("ppn = ?", payload.PPN).
		Count(&count)

	if count > 0 {
		helpers.ErrorResponse(c, 422, fmt.Sprintf("PPN %.2f sudah terdaftar", payload.PPN), nil)
		return
	}

	// CREATE
	updates := map[string]interface{}{
		"ppn": payload.PPN,
	}

	if payload.IsTaxDefault != nil {
		// Tidak boleh ubah dari true -> false
		if existingPPN.IsTaxDefault && *payload.IsTaxDefault == false {
			helpers.ErrorResponse(c, 422, "PPN default tidak boleh di nonaktifkan", nil)
			return
		}

		// Jika set jadi default
		if *payload.IsTaxDefault == true {
			// reset semua jadi false
			if err := db.Session(&gorm.Session{
				AllowGlobalUpdate: true,
			}).Model(&models.Ppn{}).
				Update("is_tax_default", false).Error; err != nil {

				helpers.ErrorResponse(c, 500, "Gagal reset default PPN", err)
				return
			}
		}

		updates["is_tax_default"] = *payload.IsTaxDefault
	}

	if err := db.Model(&existingPPN).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengupdate data PPN",
			"error": err.Error(),
		})
		return
	}

	// SUCCESS RESPONSE
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil update data PPN",
		"data": existingPPN,
	})
}
func DeletePPN(c *gin.Context) {
	ppn_id := c.Param("ppn_id")

	db := config.DB

	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "Terjadi kesalahan internal",
				"error": fmt.Sprintf("%v", r),
			})
		}
	}()

	//check data ppn
	var existingPPN models.Ppn
	if err := db.First(&existingPPN, ppn_id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Data ppn tidak ditemukan"})
		}else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message":"Gagal mengambil data ppn", "error": err.Error()})
		}

		return
	}

	//hapus data ppn
	if err := db.Delete(&existingPPN).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message":"Gagal menghapus data ppn", "error": err.Error()})		
	}

	// SUCCESS RESPONSE
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil menghapus data PPN",
		"data": existingPPN,
	})
}

