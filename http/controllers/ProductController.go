package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListProducts returns paginated products filtered by q (search name or barcode)
func ListAllProducts(c *gin.Context) {
    store_id := strings.TrimSpace(c.DefaultQuery("store_id", ""))
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    if page < 1 {
        page = 1
    }

    offset := (page - 1) * limit

    type productRow struct {
        ID      uint64 `json:"id"`
        StoreID uint64 `json:"store_id"`
        Barcode string `json:"barcode"`
        Name    string `json:"name"`
        Price   float64 `json:"price"`
        TagColor    string `json:"tag_color"`
        Quantity int64 `json:"quantity"`
        Status string `json:"status"`
        StoreName string `json:"store_name"`
        CreatedAt time.Time `json:"created_at"`
    }

    var rows []productRow

    baseWhere := "WHERE p.status = 'display'"
    args := []interface{}{}

    if store_id != "" {
        var store models.StoreProfile
        storeID, _ := strconv.Atoi(store_id)
        if err := config.DB.First(&store, storeID).Error; err != nil {
            helpers.ErrorResponse(c, 404, "store not found", err)
            return
        }

        baseWhere += " AND p.store_id = ?"
        args = append(args, store.ID)
    }

    if q != "" {
        like := "%" + q + "%"
        baseWhere += " AND (p.name LIKE ? OR p.barcode LIKE ? OR p.old_barcode LIKE ? OR s.store_name LIKE ?)"
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
			p.barcode, 
            p.name, 
			p.price, 
            p.tag_color,
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

    for i := range rows {
        rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, "Asia/Jakarta")
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
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
    if page < 1 {
        page = 1
    }
    offset := (page - 1) * limit

    type productRow struct {
        ID uint64 `json:"id"`
        StoreID uint64 `json:"store_id"`
        OldBarcode string `json:"old_barcode"`
        Barcode string `json:"barcode"`
        Name string `json:"name"`
        Price float64 `json:"price"`
        Quantity int64 `json:"quantity"`
        Status string `json:"status"`
        StoreName string `json:"store_name"`
        CreatedAt string `json:"created_at"`
    }

    var rows []productRow

    baseWhere := fmt.Sprintf("WHERE p.status = 'display' AND p.store_id = %d AND deleted_at IS NULL", *user.StoreID)
    args := []interface{}{}
    if q != "" {
        like := "%" + q + "%"
        baseWhere += " AND (p.name LIKE ? OR p.barcode LIKE ? OR p.old_barcode LIKE ? OR s.store_name LIKE ?)"
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
            OldPrice		  	float64		`json:"old_price"`
            ActualPrice			float64		`json:"actual_price"`
            Barcode       		string     	`json:"barcode" binding:"required"`
            Name          		string     	`json:"name" binding:"required"`
            Price         		float64    	`json:"price"`
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

    defer func() {
        if r := recover(); r != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
                "status":  false,
                "message": "Terjadi kesalahan internal",
                "error":   fmt.Sprintf("%v", r),
            })
        }
    }()

    // check if store token is valid
    var store models.StoreProfile
    if err := config.DB.Where("token = ?", payload.StoreToken).First(&store).Error; err != nil {
        helpers.ErrorResponse(c, 400, "invalid store token", err)
        return
    }

    // idempotency: check if any product already inserted with this document code for the store
    var exists int64
    if err := config.DB.Model(&models.MigrateProductHistory{}).
        Where("code = ?", payload.DocumentCode).
        Count(&exists).Error; err != nil {
        helpers.ErrorResponse(c, 500, "failed to check existing document", err)
        return
    }

    if exists > 0 {
        // already processed, return success (idempotent)
        helpers.ErrorResponse(c, 400, fmt.Sprintf("Migrate document dengan code %s sebelumnya sudah berhasil dimasukan", payload.DocumentCode), nil)
        return
    }
    // =========================
	// CEK DATA BARCODE DI SISTEM
	// =========================
    type conflictResult struct {
        Barcode string
        Status  string
        DeletedAt *time.Time
    }
	barcodes := make([]string, 0, len(payload.Products))
	for _, p := range payload.Products {
		barcodes = append(barcodes, p.Barcode)
	}
    //search barcode per chunk
    chunkBarcodes := chunkStrings(barcodes, 500)
    var conflicts []conflictResult

	for _, chunk := range chunkBarcodes {
        var temp []conflictResult
        err := config.DB.
            Model(&models.Product{}).
            Select("barcode, status, deleted_at").
            Where("barcode IN ?", chunk).
            Where(`
                status = 'sale' 
                OR (deleted_at IS NULL AND status = 'display')
            `).
            Scan(&temp).Error

        if err != nil {
            helpers.ErrorResponse(c, 500, "failed check conflict", err)
            return
        }

        conflicts = append(conflicts, temp...)
    }
    
    //Pisah produk yang status sale dan masih aktif
	var soldBarcodes []string
    var activeBarcodes []string
    for _, c := range conflicts {
        if c.Status == "sale" {
            soldBarcodes = append(soldBarcodes, c.Barcode)
        } else if c.Status == "display" && c.DeletedAt == nil {
            activeBarcodes = append(activeBarcodes, c.Barcode)
        }
    }

    //jika ada product aktif atau sudah sale → STOP
    if len(soldBarcodes) > 0 || len(activeBarcodes) > 0 {
        c.JSON(400, gin.H{
            "status":          false,
            "message":         "Terdapat produk yang tidak bisa diproses",
            "sold_barcodes":   soldBarcodes,
            "active_barcodes": activeBarcodes,
        })
        return
    }

    // ====================
    // PROSES DATA PRODUK
    // ====================
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

    // Batch uptsert data product
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
            if err := tx.Clauses(clause.OnConflict{
                Columns: []clause.Column{
                    {Name: "barcode"},
                },
                DoUpdates: clause.Assignments(map[string]interface{}{
                    "migrate_id":       gorm.Expr("VALUES(migrate_id)"),
                    "code_document":    gorm.Expr("VALUES(code_document)"),
                    "old_barcode":      gorm.Expr("VALUES(old_barcode)"),
                    "old_price":        gorm.Expr("VALUES(old_price)"),
                    "actual_price":     gorm.Expr("VALUES(actual_price)"),
                    "name":             gorm.Expr("VALUES(name)"),
                    "price":            gorm.Expr("VALUES(price)"),
                    "quantity":         gorm.Expr("VALUES(quantity)"),
                    "status":           "display",
                    "tag_color":        gorm.Expr("VALUES(tag_color)"),
                    "is_so":            gorm.Expr("VALUES(is_so)"),
                    "is_extra_product": gorm.Expr("VALUES(is_extra_product)"),
                    "user_so":          gorm.Expr("VALUES(user_so)"),
                    "deleted_at": nil,
                }),
            }).Create(&batch).Error; err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 500, "failed upsert product", err)
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
func DeleteProdukBKL(c *gin.Context) {
    var payload struct {
        ProductBarcode []string `json:"product_barcode" binding:"required,dive,required"`
    }

    defer func() {
        if r := recover(); r != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
                "status":  false,
                "message": "Terjadi kesalahan internal",
                "error":   fmt.Sprintf("%v", r),
            })
        }
    }()

    if err := c.ShouldBindJSON(&payload); err != nil {
        errors := response.FormatValidationError(err)

        c.JSON(400, gin.H{
            "message": "Invalid payload",
            "errors":  errors,
        })
        return
    }

    if len(payload.ProductBarcode) == 0 {
        c.JSON(400, gin.H{
            "message": "Invalid payload",
            "errors":  []string{"product_barcode must contain at least one barcode"},
        })
        return
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if tx.Error != nil {
        helpers.ErrorResponse(c, 500, "failed to start transaction", tx.Error)
        return
    }

    const batchSize = 100

    type ProductLite struct {
        ID      uint64
        Barcode string
        Status  string
    }

    var products []ProductLite

    // =========================
    // GET PRODUCTS
    // =========================
    for i := 0; i < len(payload.ProductBarcode); i += batchSize {
        end := i + batchSize
        if end > len(payload.ProductBarcode) {
            end = len(payload.ProductBarcode)
        }

        batch := payload.ProductBarcode[i:end]

        var temp []ProductLite
        if err := tx.Model(&models.Product{}).
            Select("id, barcode, status").
            Where("barcode IN ?", batch).
            Find(&temp).Error; err != nil {

            tx.Rollback()
            helpers.ErrorResponse(c, 500, "failed to query products", err)
            return
        }

        products = append(products, temp...)
    }

    if len(products) == 0 {
        tx.Rollback()
        helpers.ErrorResponse(c, 404, "no products found", nil)
        return
    }

    // =========================
    // MAP PRODUCT
    // =========================
    productMap := make(map[string]ProductLite)
    var productIDs []uint64

    for _, p := range products {
        productMap[p.Barcode] = p
        productIDs = append(productIDs, p.ID)
    }

    // =========================
    // CHECK FK USAGE
    // =========================
    // usedMap := make(map[uint64]struct{})
    // var trxUsed []uint64
    // if err := tx.Model(&models.TransactionItem{}).
    //     Where("product_id IN ?", productIDs).
    //     Pluck("DISTINCT product_id", &trxUsed).Error; err != nil {

    //     tx.Rollback()
    //     helpers.ErrorResponse(c, 500, "failed check transaction items", err)
    //     return
    // }

    // for _, id := range trxUsed {
    //     usedMap[id] = struct{}{}
    // }

    // =========================
    // SPLIT DATA
    // =========================
    var (
        deletableIDs []uint64
        missing      []string
        skippedSale  []string
        // skippedUsed  []string
    )

    for _, b := range payload.ProductBarcode {
        p, ok := productMap[b]

        if !ok {
            missing = append(missing, b)
            continue
        }

        if p.Status == "sale" {
            skippedSale = append(skippedSale, b)
            continue
        }

        // if _, used := usedMap[p.ID]; used {
        //     skippedUsed = append(skippedUsed, b)
        //     continue
        // }

        deletableIDs = append(deletableIDs, p.ID)
    }

    // =========================
    // DELETE SAFE DATA
    // =========================
    deletedCount := 0

    for i := 0; i < len(deletableIDs); i += batchSize {
        end := i + batchSize
        if end > len(deletableIDs) {
            end = len(deletableIDs)
        }

        batch := deletableIDs[i:end]

        res := tx.Where("id IN ?", batch).
            Delete(&models.Product{})

        if res.Error != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "failed to delete products", res.Error)
            return
        }

        deletedCount += int(res.RowsAffected)
    }

    // =========================
    // COMMIT
    // =========================
    if err := tx.Commit().Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "commit failed", err)
        return
    }

    // =========================
    // RESPONSE
    // =========================
    data := gin.H{
        "requested_count": len(payload.ProductBarcode),
        "deleted_count":   deletedCount,

        "skipped_sale_count": len(skippedSale),
        // "skipped_used_count": len(skippedUsed),
        "missing_count":      len(missing),

        "skipped_sale": skippedSale,
        // "skipped_used": skippedUsed,
        "missing":      missing,
    }

    c.JSON(http.StatusOK, response.Success("Produk BKL berhasil diproses", data))
}

//====================== helper ======================

func chunkStrings(data []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(data); i += size {
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	return chunks
}