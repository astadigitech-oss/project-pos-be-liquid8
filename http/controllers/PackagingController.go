package controllers

import (
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
)

func ListPackagings(c *gin.Context) {
	type PackagingResponse struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Price float64 `json:"price"`
	}
	var packagings []PackagingResponse
	if err := config.DB.Model(&models.Packaging{}).Where("store_id IS NULL").Scan(&packagings).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil data kemasan", err)
		return
	}

	c.JSON(200, response.Success("Daftar kemasan", packagings))
}