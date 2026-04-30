package controllers

import (
	"errors"
	"fmt"
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"

	"net/http"

	"github.com/gin-gonic/gin"
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
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors": gin.H{
				"ppn": "PPN sudah terdaftar",
			},
		})
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

	// UNIQUE CHECK
	var count int64
	db.Model(&models.Ppn{}).
		Where("id != ?", existingPPN.ID).
		Where("ppn = ?", payload.PPN).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"message": "Validasi gagal",
			"errors": gin.H{
				"ppn": "PPN sudah terdaftar",
			},
		})
		return
	}

	// CREATE
	updates := map[string]interface{}{
		"ppn": payload.PPN,
	}

	if payload.IsTaxDefault != nil {
		updates["is_tax_default"] = payload.IsTaxDefault

		if err := db.Session(&gorm.Session{
			AllowGlobalUpdate: true,
		}).Model(&models.Ppn{}).Update("is_tax_default", false).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
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

