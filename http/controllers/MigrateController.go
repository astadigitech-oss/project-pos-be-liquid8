package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListPendingMigrateHistories returns pending migrate product histories (status = 'pending')
func ListPendingMigrateHistories(c *gin.Context) {
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    if page < 1 { page = 1 }
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
    offset := (page-1)*limit

    db := config.DB.Model(&models.MigrateProductHistory{}).Where("status = ?", "pending")
    if q != "" {
        like := "%" + q + "%"
        // search by code or user or store name via join
        db = db.Joins("LEFT JOIN store_profiles s ON s.id = migrate_product_histories.store_id").Where("migrate_product_histories.code LIKE ? OR migrate_product_histories.user LIKE ? OR s.store_name LIKE ?", like, like, like)
    }

    var total int64
    if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to count pending migrate histories", err)
        return
    }

    var rows []models.MigrateProductHistory
    if err := db.Limit(limit).Offset(offset).Order("created_at desc").Find(&rows).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to fetch pending migrate histories", err)
        return
    }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    c.JSON(http.StatusOK, response.Success("pending migrate histories", gin.H{"data": rows, "pagination": pagination}))
}
