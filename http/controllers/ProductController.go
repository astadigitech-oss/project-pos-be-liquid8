package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
)

// ListProducts returns paginated products filtered by q (search name or barcode)
func ListAllProducts(c *gin.Context) {
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    if page < 1 {
        page = 1
    }
    limit := 30
    offset := (page - 1) * limit

    type productRow struct {
        ID uint64 `json:"id"`
        StoreID uint64 `json:"store_id"`
        Barcode string `json:"barcode"`
        Name string `json:"name"`
        Price float64 `json:"price"`
        Quantity int64 `json:"quantity"`
        Status string `json:"status"`
        StoreName string `json:"store_name"`
        CreatedAt string `json:"created_at"`
    }

    var rows []productRow

    baseWhere := "WHERE p.status = 'display'"
    args := []interface{}{}
    if q != "" {
        like := "%" + q + "%"
        baseWhere += "AND (p.name LIKE ? OR p.barcode LIKE ? OR s.store_name LIKE ?)"
        args = append(args, like, like, like)
    }

    // count
    var totalData int64
    countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM products p JOIN store_profiles s ON s.id = p.store_id %s`, baseWhere)
    if len(args) > 0 {
        if err := config.DB.Raw(countSQL, args...).Scan(&totalData).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to count products", err)
            return
        }
    } else {
        if err := config.DB.Raw(countSQL).Scan(&totalData).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to count products", err)
            return
        }
    }

    // data
    dataSQL := fmt.Sprintf(`
		SELECT 
			p.id, 
			p.store_id, 
			p.barcode, p.name, 
			p.price, 
			p.quantity, 
			p.status, 
			COALESCE(s.store_name, '') as store_name, 
			p.created_at 
		FROM products p 
		JOIN store_profiles s ON s.id = p.store_id %s ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to fetch products", err)
        return
    }

    lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

    c.JSON(http.StatusOK, gin.H{
        "status": true,
        "message": "List products",
        "resource": gin.H{
            "data": rows,
            "pagination": pagination,
        },
    })
}
func ListProductsOfStore(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)

    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    if page < 1 {
        page = 1
    }
    limit := 30
    offset := (page - 1) * limit

    type productRow struct {
        ID uint64 `json:"id"`
        StoreID uint64 `json:"store_id"`
        Barcode string `json:"barcode"`
        Name string `json:"name"`
        Price float64 `json:"price"`
        Quantity int64 `json:"quantity"`
        Status string `json:"status"`
        StoreName string `json:"store_name"`
        CreatedAt string `json:"created_at"`
    }

    var rows []productRow

    baseWhere := fmt.Sprintf("WHERE p.status = 'display' AND p.store_id = %d ", *user.StoreID)
    args := []interface{}{}
    if q != "" {
        like := "%" + q + "%"
        baseWhere += "AND (p.name LIKE ? OR p.barcode LIKE ? OR s.store_name LIKE ?)"
        args = append(args, like, like, like)
    }

    // count
    var totalData int64
    countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM products p JOIN store_profiles s ON s.id = p.store_id %s`, baseWhere)
    if len(args) > 0 {
        if err := config.DB.Raw(countSQL, args...).Scan(&totalData).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to count products", err)
            return
        }
    } else {
        if err := config.DB.Raw(countSQL).Scan(&totalData).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to count products", err)
            return
        }
    }

    // data
    dataSQL := fmt.Sprintf(`
		SELECT 
			p.id, 
			p.store_id, 
			p.barcode, p.name, 
			p.price, 
			p.quantity, 
			p.status, 
			COALESCE(s.store_name, '') as store_name, 
			p.created_at 
		FROM products p 
		JOIN store_profiles s ON s.id = p.store_id %s ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to fetch products", err)
        return
    }

    lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

    c.JSON(http.StatusOK, gin.H{
        "status": true,
        "message": "List products",
        "resource": gin.H{
            "data": rows,
            "pagination": pagination,
        },
    })
}

