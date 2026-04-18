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

// ListStores returns paginated store profiles
func ListStores(c *gin.Context) {
	q := strings.TrimSpace(c.DefaultQuery("q", ""))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	offset := (page - 1) * limit

	type Result struct {
		models.StoreProfile
		TotalProducts int64   `json:"total_products"`
		TotalSales    float64 `json:"total_sales"`
	}

	var results []Result

	db := config.DB.Model(&models.StoreProfile{}).
		Select(`
			store_profiles.*,
			COALESCE(p.total_products, 0) as total_products,
			COALESCE(t.total_sales, 0) as total_sales
		`).
		Joins(`
			LEFT JOIN (
				SELECT store_id, COUNT(*) as total_products
				FROM products
				WHERE status = 'display'
				GROUP BY store_id
			) p ON p.store_id = store_profiles.id
		`).
		Joins(`
			LEFT JOIN (
				SELECT store_id, SUM(total_amount) as total_sales
				FROM transactions
				WHERE status = 'done'
				GROUP BY store_id
			) t ON t.store_id = store_profiles.id
		`)

	if q != "" {
		like := "%" + q + "%"
		db = db.Where("store_name LIKE ? OR phone LIKE ? OR address LIKE ?", like, like, like)
	}

	// count total
	var total int64
	if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count stores", err)
		return
	}

	// fetch data
	if err := db.Limit(limit).Offset(offset).Find(&results).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to fetch stores", err)
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(results), int(total))

	c.JSON(http.StatusOK, response.Success("List stores", gin.H{
		"data": results,
		"pagination": pagination,
	}))
}

func DetailStore(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    var store models.StoreProfile
    if err := config.DB.First(&store, id).Error; err != nil {
        helpers.ErrorResponse(c, 404, "store not found", err)
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": true, "resource": store})
}

// CreateStore creates a store profile
func CreateStore(c *gin.Context) {
    var payload models.StoreProfile
    if err := c.ShouldBindJSON(&payload); err != nil {
        helpers.ErrorResponse(c, 400, "invalid payload", err)
        return
    }
    if err := config.DB.Create(&payload).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to create store", err)
        return
    }
    c.JSON(http.StatusCreated, gin.H{"status": true, "resource": payload})
}

// UpdateStore updates a store profile
func UpdateStore(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    var store models.StoreProfile
    if err := config.DB.First(&store, id).Error; err != nil {
        helpers.ErrorResponse(c, 404, "store not found", err)
        return
    }
    var payload models.StoreProfile
    if err := c.ShouldBindJSON(&payload); err != nil {
        helpers.ErrorResponse(c, 400, "invalid payload", err)
        return
    }
    // update fields
    store.StoreName = payload.StoreName
    store.Phone = payload.Phone
    store.Address = payload.Address
    store.Timezone = payload.Timezone
    store.Token = payload.Token

    if err := config.DB.Save(&store).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to update store", err)
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": true, "resource": store})
}

// DeleteStore deletes a store profile
func DeleteStore(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    if err := config.DB.Delete(&models.StoreProfile{}, id).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to delete store", err)
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": true, "message": "store deleted"})
}

func ListStoresForSync(c *gin.Context) {
    var stores []models.StoreProfile

    if err := config.DB.Model(&models.StoreProfile{}).Find(&stores).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to fetch stores", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("List stores", stores))
}
