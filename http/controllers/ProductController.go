package controllers

import (
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
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    if page < 1 {
        page = 1
    }
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
func ReceiveMigrateDocument(c *gin.Context) {
    var payload struct {
        DocumentCode string `json:"document_code" binding:"required"`
        StoreToken   string `json:"store_token" binding:"required"`
        Products     []struct {
            CodeDocument  		*string     `json:"code_document"`
            OldBarcode		 	*string  	`json:"old_barcode"`
            OldPrice		  	float64		`json:"old_price" binding:"required"`
            ActualPrice			float64		`json:"actual_price" binding:"required"`
            Barcode       		string     	`json:"barcode" binding:"required"`
            Name          		string     	`json:"name" binding:"required"`
            Price         		float64    	`json:"price" binding:"required"`
            Quantity      		int64      	`json:"quantity" binding:"required"`
            Status        		string     	`json:"status" binding:"required"`
            TagColor    		string     	`json:"tag_color" binding:"required"`
            IsSo          		*string     `json:"is_so"`
            IsExtraProduct    	*bool     	`json:"is_extra_product"`
            UserSo        		*uint64     `json:"user_so"`
        } `json:"products" binding:"required,dive,required"`
    }

    if err := c.ShouldBindJSON(&payload); err != nil {
        errors := response.FormatValidationError(err)

        c.JSON(400, gin.H{
            "message": "Invalid payload",
            "errors":  errors,
        })
        return
    }

    // check if store token is valid
    var store models.StoreProfile
    if err := config.DB.Where("token = ?", payload.StoreToken).First(&store).Error; err != nil {
        helpers.ErrorResponse(c, 400, "invalid store token", err)
        return
    }

    // idempotency: check if any product already inserted with this document code for the store
    var exists int64
    if err := config.DB.Model(&models.MigrateProductHistory{}).
        Where("code = ? AND store_id = ?", payload.DocumentCode, store.ID).
        Count(&exists).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to check existing document", err)
        return
    }

    if exists > 0 {
        // already processed, return success (idempotent)
        helpers.ErrorResponse(c, 400, fmt.Sprintf("Migrate document dengan code %s sebelumnya sudah berhasil dimasukan", payload.DocumentCode), nil)
        return
    }

    // start transaction
    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if tx.Error != nil {
        helpers.ErrorResponse(c, 500, "failed to start transaction", tx.Error)
        return
    }

    // insert migrate history
    hist := models.MigrateProductHistory{
        StoreID:        uint64(store.ID),
        Code:           &payload.DocumentCode,
        User:          "wms",
        TotalProduct:  len(payload.Products),
        TypeMigration: "IN",
    }

    if err := tx.Create(&hist).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "failed to insert migrate history", err)
        return
    }

    // insert products
    totalQuantity := int64(0)
    totalPrice := float64(0)
    batchSize := 100
    var batch []models.Product
    for i, p := range payload.Products {
        totalPrice += p.Price
        totalQuantity += p.Quantity

        isExtra := false
        if p.IsExtraProduct != nil {isExtra = *p.IsExtraProduct}

        batch = append(batch, models.Product{
            StoreID:  uint64(store.ID),
            MigrateID: &hist.ID,
            CodeDocument: p.CodeDocument,
            OldBarcode: p.OldBarcode,
            OldPrice:   p.OldPrice,
            ActualPrice: p.ActualPrice,
            Barcode:   p.Barcode,
            Name:      p.Name,
            Price:     p.Price,
            Quantity:  p.Quantity,
            Status:    "display",
            TagColor: p.TagColor,
            IsSo: p.IsSo,
            IsExtraProduct: isExtra,
            UserSo: p.UserSo,
        })

        if len(batch) == batchSize || i == len(payload.Products)-1 {
            if err := tx.CreateInBatches(batch, batchSize).Error; err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 500, "failed to insert product", err)
                return
            }
            batch = nil // reset
        }
    }

    if err := tx.Model(&hist).Updates(map[string]interface{}{
        "total_quantity": totalQuantity,
        "total_price":    totalPrice,
    }).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "failed to update migrate history", err)
        return
    }

    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Migrate product berhasil dilakukan", nil))
}
