package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListMigrateHistories returns all migrate product histories
func ListMigrateHistories(c *gin.Context) {
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    if page < 1 { page = 1 }
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
    offset := (page-1)*limit

    type resultFormat struct {
        ID             uint64 `json:"id"`
        StoreID        uint64 `json:"store_id"`
        StoreName      string `json:"store_name"`
        Code           string `json:"code"`
        User           string `json:"user"`
        TotalProduct   int64  `json:"total_product"`
        TotalPrice    float64 `json:"total_price"`
        CreatedAt     time.Time `json:"created_at"`
    }

    db := config.DB.Table("migrate_product_histories mph").Select(`
        mph.id, 
        mph.store_id, 
        COALESCE(s.store_name, '') AS store_name, 
        mph.code, 
        mph.user, 
        mph.total_product, 
        mph.total_price, 
        mph.created_at
    `).Joins("LEFT JOIN store_profiles s ON s.id = mph.store_id")
    if q != "" {
        like := "%" + q + "%"
        // search by code or user or store name via join
        db = db.Where("mph.code LIKE ? OR mph.user LIKE ? OR s.store_name LIKE ?", like, like, like)
    }

    var total int64
    if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to count migrate histories", err)
        return
    }

    var rows []resultFormat
    if err := db.Limit(limit).Offset(offset).Order("mph.created_at desc").Find(&rows).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to fetch pending migrate histories", err)
        return
    }

    for i := range rows {
        rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, "Asia/Jakarta")
    }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    c.JSON(http.StatusOK, response.Success("pending migrate histories", gin.H{"data": rows, "pagination": pagination}))
}
