package controllers

import (
	"errors"
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
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

//=========================== cart item ======================
func AddToCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)

	defer func() {
		if r := recover(); r != nil {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Terjadi kesalahan internal",
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

    type payload struct {
        ProductBarcode string  `json:"product_barcode" binding:"required"`
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
			case "ProductBarcode":
				if e.Tag() == "required" {
					errorsMap["product_barcode"] = "Product barcode wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

    storeID := uint64(0)
    if user.StoreID != nil {
        storeID = *user.StoreID
    }else {
		helpers.ErrorResponse(c, 400, "User tidak memiliki store_id", nil)
		return
	}

    // load product
    var product models.Product
    if err := config.DB.Where("barcode = ? AND store_id = ? AND deleted_at IS NULL", p.ProductBarcode, storeID).First(&product).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Product tidak ditemukan", err)
        return
    }

	if product.Status == "sale" {
		helpers.ErrorResponse(c, 422, "Barang sudah discan", nil)
		return
	}

    // create cart item
    cart := models.CartItem{
        StoreID: storeID,
        UserID:  uint64(user.ID),
        ProductID: product.ID,
        ProductName: product.Name,
        Quantity: int64(product.Quantity),
        Price: product.Price,
        DiscountPrice: 0,
        Subtotal: product.Price,
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if err := tx.Create(&cart).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to add to cart", err)
        return
    }

    // update product status
    if err := tx.Model(&product).Update("status", "sale").Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to update product", err)
        return
    }

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Added to cart", cart))
}
func RemoveItemCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    storeID := uint64(0)
    if user.StoreID == nil {
        helpers.ErrorResponse(c, 400, "User does not have store ID", nil)
        return
    }
    storeID = *user.StoreID
    cartID := c.Param("cart_id")

    var cartItem models.CartItem
    if err := config.DB.Where("id = ? AND user_id = ? AND store_id = ?", cartID, user.ID, storeID).First(&cartItem).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Cart item not found", err)
        return
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    //update data product back to display
    if err := tx.Model(&models.Product{}).Where("id = ?", cartItem.ProductID).Update("status", "display").Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to update product status", err)
        return
    }
    // delete cart item
    if err := tx.Delete(&cartItem).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to remove cart item", err)
        return
    }

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Cart item removed", nil))
}
func PendingCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    var payload struct {
		MemberID	uint64	`json:"member_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}
		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "MemberID":
				if e.Tag() == "required" {
					errorsMap["member_id"] = "Member ID wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

    storeID := uint64(0)
    if user.StoreID != nil { 
		storeID = *user.StoreID 
	}else {
		helpers.ErrorResponse(c, 403, "User tidak memiliki store ID", nil)
		return
	}

    //load data member
    var member models.Member
    if err := config.DB.Where("id = ?", payload.MemberID).First(&member).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "Member not found", nil)
        } else {
            helpers.ErrorResponse(c, 500, "Failed to load member", err)
        }
        return
    }

    // generate a keep code (simple UUID short)
    keep, err := helpers.GeneratePendingKeepCode(config.DB, *user.StoreID); 
	if err != nil {
		helpers.ErrorResponse(c, 422, "Gagal generate keep code", err)
		return
	}

    // update cart items without keep_code for this user/store
    if err := config.DB.Model(&models.CartItem{}).
        Where("user_id = ? AND store_id = ? AND (keep_code = '' OR keep_code IS NULL)", user.ID, storeID).
        Updates(map[string]interface{}{
            "keep_code": keep,
            "member_id": payload.MemberID,
        }).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to pend cart", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Cart pending created", nil))
}
func ListPending(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    storeID := uint64(0)
    if user.StoreID == nil {
        helpers.ErrorResponse(c, 403, "User does not have store ID", nil)
        return
    }
    storeID = *user.StoreID

    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
    if page < 1 { page = 1 }
    offset := (page-1)*limit

    type pendingGroup struct {
        CustomerName string  `json:"customer_name"`
        KeepCode     string  `json:"keep_code"`
        ItemCount    int64   `json:"item_count"`
        Total        float64 `json:"total"`
    }

    var groups []pendingGroup

    // build where and args
    baseWhere := "WHERE ci.user_id = ? AND ci.store_id = ? AND ci.keep_code IS NOT NULL AND ci.keep_code != ''"
    args := []interface{}{user.ID, storeID}
    if q != "" {
        baseWhere += " AND m.name LIKE ?"
        args = append(args, "%"+q+"%")
    }

    // count distinct keep_code
    var total int64
    countSQL := "SELECT COUNT(DISTINCT ci.keep_code) FROM cart_items ci JOIN members m ON ci.member_id = m.id " + baseWhere
    if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to count pending groups", err)
        return
    }

    dataSQL := fmt.Sprintf(`
        SELECT 
            MAX(m.name) AS customer_name,
            ci.keep_code,
            COUNT(*) AS item_count,
            COALESCE(SUM(ci.subtotal),0) AS total
        FROM cart_items ci 
        JOIN members m ON ci.member_id = m.id 
        %s
        GROUP BY ci.keep_code
        ORDER BY MAX(ci.created_at) DESC
        LIMIT ? OFFSET ?
    `, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&groups).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to list pending groups", err)
        return
    }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(groups), int(total))

    c.JSON(http.StatusOK, response.Success("List Pending transactions", gin.H{"data": groups, "pagination": pagination}))
}
func ResumePendingCheck(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    storeID := uint64(0)
    if user.StoreID == nil {
		helpers.ErrorResponse(c, 400, "User does not have store ID", nil)
		return
	}
	storeID = *user.StoreID

    keep := c.Param("keep_code")
	var existing models.CartItem
    if err := config.DB.Where("keep_code = ? AND store_id = ?", keep, storeID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 404, "Tidak ada cart item yang ditemukan", nil)
		}else {
			helpers.ErrorResponse(c, 500, "Internal server error", err)
		}
		return
	}

    // check current active cart (without keep_code)
    var count int64
    if err := config.DB.Model(&models.CartItem{}).
        Where("user_id = ? AND store_id = ? AND (keep_code = '' OR keep_code IS NULL)", user.ID, storeID).
        Count(&count).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to check cart", err)
        return
    }

    if count > 0 {
        helpers.ErrorResponse(c, 422, "Kosongkan/Pending cart terlebih dahulu", nil)
        return
    }

	//update cart item
	if err := config.DB.Model(&models.CartItem{}).Where("keep_code = ?", keep).Update("keep_code", nil).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Internal server error", err)
		return
	}

	var cart_items []models.CartItem
	if err := config.DB.Where("user_id = ? and store_id = ? AND (keep_code = '' OR keep_code IS NULL)", user.ID, storeID).Find(&cart_items).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Internal server error", err)
		return
	}

    c.JSON(http.StatusOK, response.Success("Resume transaksi berhasil", cart_items))
}
func GetCurrentCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    storeID := uint64(0)
    if user.StoreID != nil { storeID = *user.StoreID } else { helpers.ErrorResponse(c, 422, "User tidak memiliki store ID", nil); return }

    type cartItemResponse struct {
        ID            uint64  `json:"id"`
        StoreID       uint64  `json:"store_id"`
        MemberID      *uint64 `json:"member_id"`
        UserID        uint64  `json:"user_id"`
        ProductID     uint64  `json:"product_id"`
        Barcode       string  `json:"barcode"`
        KeepCode      *string `json:"keep_code"`
        ProductName   string  `json:"product_name"`
        Quantity      int64   `json:"quantity"`
        Price         float64 `json:"price"`
        DiscountPrice float64 `json:"discount_price"`
        Subtotal      float64 `json:"subtotal"`
    }

    var items []cartItemResponse
    err := config.DB.
        Table("cart_items").
        Select(`
            cart_items.id,
            cart_items.store_id,
            cart_items.member_id,
            cart_items.user_id,
            cart_items.product_id,
            products.barcode,
            cart_items.keep_code,
            products.name as product_name,
            cart_items.quantity,
            cart_items.price,
            cart_items.discount_price,
            cart_items.subtotal
        `).
        Joins("LEFT JOIN products ON products.id = cart_items.product_id").
        Where("cart_items.user_id = ? AND cart_items.store_id = ? AND (cart_items.keep_code IS NULL OR cart_items.keep_code = '')",
            user.ID, storeID).
        Scan(&items).Error

    if err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load current cart", err)
        return
    }

    var ppn models.Ppn
    if err := config.DB.Where("is_tax_default = ?", true).First(&ppn).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "PPN tidak ditemukan", nil)
        } else {
            helpers.ErrorResponse(c, 500, "Internal server error", err)
        }
        return     
    }

    var totalSubtotal, totalAmount float64
    var totalQuantity int64
    for _, it := range items {
        totalSubtotal += it.Subtotal
        totalQuantity += it.Quantity
    }

    ppn_price := math.Round(totalSubtotal * (ppn.Ppn / 100))
    totalBelanja := math.Round(totalSubtotal + ppn_price)
    totalAmount = helpers.RoundTo500(int(totalBelanja))
    pembulatan := totalAmount - totalBelanja

    payload := gin.H{
        "items": items,
        "subtotal": totalSubtotal,
        "ppn": gin.H{
            "tax": ppn.Ppn,
            "amount": ppn_price,
        },
        "total_amount": totalAmount,
        "pembulatan": pembulatan,
    }

    c.JSON(http.StatusOK, response.Success("Current cart", payload))
}
func RemoveCartByKeepCode(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    storeID := uint64(0)
    if user.StoreID != nil { storeID = *user.StoreID } else { helpers.ErrorResponse(c, 400, "User tidak memiliki store ID", nil); return }

    keep := c.Param("keep_code")
    if strings.TrimSpace(keep) == "" {
        helpers.ErrorResponse(c, 400, "keep_code required", nil)
        return
    }

    // load cart items to know affected products
    var items []models.CartItem
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code = ?", user.ID, storeID, keep).Find(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart items", err)
        return
    }

    if len(items) == 0 {
        helpers.ErrorResponse(c, 404, "No cart items found for given keep_code", nil)
        return
    }

    // collect product ids
    prodIDs := make([]uint64, 0, len(items))
    for _, it := range items {
        prodIDs = append(prodIDs, it.ProductID)
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if err := tx.Where("user_id = ? AND store_id = ? AND keep_code = ?", user.ID, storeID, keep).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart items", err)
        return
    }

    // reset product status to 'display' for affected products
    if len(prodIDs) > 0 {
        if err := tx.Model(&models.Product{}).Where("id IN ?", prodIDs).Updates(map[string]interface{}{"status": "display"}).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to update product status", err)
            return
        }
    }

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Cart items removed", nil))
}
func EmptyCurrentCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)

    // load cart items
    var items []models.CartItem
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, *user.StoreID).Find(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart items", err)
        return
    }

    if len(items) == 0 {
        helpers.ErrorResponse(c, 400, "Cart item kosong", nil)
        return
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    // delete cart items
    if err := tx.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, *user.StoreID).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart items", err)
        return
    }

    // collect product ids
    prodIDs := make([]uint64, 0, len(items))
    for _, it := range items {
        prodIDs = append(prodIDs, it.ProductID)
    }

    // reset product status to 'display' for affected products
    if len(prodIDs) > 0 {
        if err := tx.Model(&models.Product{}).Where("id IN ?", prodIDs).Updates(map[string]interface{}{"status": "display"}).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to update product status", err)
            return
        }
    }

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Berhasil mengosongkan keranjang", nil))
}

//=========================== Transaksi ======================

func CheckoutTransaction(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    shift := c.MustGet("shift_active").(models.Shift)

    defer func() {
		if r := recover(); r != nil {
			c.JSON(500, gin.H{
				"success": false,
				"message": "Internal server error",
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

    // storeID := uint64(0)
    if user.Store == nil {
		helpers.ErrorResponse(c, 400, "User does not have store ID", nil)
		return
	}
    storeID := uint64(user.Store.ID)

    type payload struct {
		MemberID	uint64	`json:"member_id" binding:"required"`
        // Tax float64 `json:"tax" binding:"gte=0,max=100"`
        PaymentMethod string `json:"payment_method" binding:"required,oneof=cash transfer qris"`
        PaidAmount float64 `json:"paid_amount" binding:"required,gte=0"`
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
			case "MemberID":
				if e.Tag() == "required" {
					errorsMap["member_id"] = "Member ID wajib diisi"
				}
			case "PaymentMethod":
				if e.Tag() == "required" {
					errorsMap["payment_method"] = "Payment method wajib diisi"
				}else {
					errorsMap["payment_method"] = "Payment method harus cash, transfer atau qris"
				}
			case "PaidAmount":
				if e.Tag() == "required" {
					errorsMap["paid_amount"] = "Paid amount wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

    //load data member
    var member models.Member
    if err := config.DB.Where("id = ?", p.MemberID).First(&member).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "Member not found", nil)
        } else {
            helpers.ErrorResponse(c, 500, "Failed to load member", err)
        }
        return
    }

    // load ppn
    var ppn models.Ppn
    if err := config.DB.Where("is_tax_default = ?", true).First(&ppn).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "default ppn belum di set", nil)
        }else {
            helpers.ErrorResponse(c, 500, "Failed to load PPN", err)
        }
        return
    }

    // load cart items
    var items []models.CartItem
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, storeID).Find(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart items", err)
        return
    }

    if len(items) == 0 {
        helpers.ErrorResponse(c, 400, "Cart item kosong", nil)
        return
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    // create transaction
    invoice, err := helpers.GenerateInvoice(tx, storeID)
	if err != nil {
        tx.Rollback()
		helpers.ErrorResponse(c, 422, "Gagal generate invoice", err)
		return
	}

    now, err := helpers.GetCurentTime(user.Store.Timezone)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
    
    tr := models.Transaction{
        StoreID: storeID,
        UserID: uint64(user.ID),
        ShiftID: shift.ID,
        Invoice: invoice,
        MemberID: uint64(member.ID),
        TotalItem: len(items),
        Tax: ppn.Ppn,
        PaidAmount: p.PaidAmount,
        PaymentMethod: p.PaymentMethod,
        Status: "done",
        CreatedAt: now.UTC(),
        UpdatedAt: now.UTC(),
    }

    if err := tx.Create(&tr).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to create transaction", err)
        return
    }

    totalQty := 0
	subTotal := float64(0)
    type txRowModel struct {
        Name     string  `json:"product_name"`
        Price    float64 `json:"price"`
    }
    var txRow []txRowModel
    var productIDs []uint64
    // migrate items
    for _, it := range items {
        ti := models.TransactionItem{
            StoreID: storeID,
            TransactionID: tr.ID,
            ProductID: it.ProductID,
            ProductName: it.ProductName,
            Quantity: int(it.Quantity),
            Price: it.Price,
            DiscountPrice: it.DiscountPrice,
            Subtotal: it.Subtotal,
        }

        productIDs = append(productIDs, it.ProductID)
        txRow = append(txRow, txRowModel{
            Name:     it.ProductName,
            Price:    it.Price,
        })

        if err := tx.Create(&ti).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to create transaction item", err)
            return
        }

        totalQty += int(it.Quantity)
		subTotal += it.Subtotal
    }

    ppnAmount := math.Round(subTotal * (ppn.Ppn / 100))
	totalTransaction := math.Round(subTotal + ppnAmount)
    totalAmount := helpers.RoundTo500(int(totalTransaction))
    roundedPrice := totalAmount - totalTransaction
	changeAmount := tr.PaidAmount - totalAmount
	if changeAmount < 0 {
		tx.Rollback()
		helpers.ErrorResponse(c, 422, fmt.Sprintf("Paid amount (%.2f) tidak boleh kurang dari total amount (%.2f)", p.PaidAmount, totalAmount), nil)
		return
	}
    // update transaction totals
    if err := tx.Model(&tr).Updates(map[string]interface{}{
        "tax_price": ppnAmount,
        "rounded_price": roundedPrice,
        "total_quantity": totalQty,
		"total_amount": totalAmount,
		"change_amount": changeAmount,
		"subtotal": subTotal,
    }).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to update transaction totals", err)
        return
    }

    // delete cart items
    if err := tx.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, storeID).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to clear cart", err)
        return
    }

    //update product to sale
    if len(productIDs) > 0 {
        if err := tx.Model(&models.Product{}).
            Where("id IN ?", productIDs).
            Update("status", "sale").Error; err != nil {

            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to update status product", err)
            return
        }
    }

    // INCREMENTAL SHIFT UPDATE
    var cash, transfer, qris float64
	switch p.PaymentMethod {
	case "cash":
		cash = totalAmount
	case "transfer":
		transfer = totalAmount
	case "qris":
		qris = totalAmount
	}

	if err := tx.Model(&models.Shift{}).
		Where("id = ?", shift.ID).
		Updates(map[string]interface{}{
			"total_cash":     gorm.Expr("total_cash + ?", cash),
			"total_transfer": gorm.Expr("total_transfer + ?", transfer),
			"total_qris":     gorm.Expr("total_qris + ?", qris),
			"total_tax":      gorm.Expr("total_tax + ?", ppnAmount),
			"subtotal":       gorm.Expr("subtotal + ?", subTotal),
			"expected_amount": gorm.Expr("expected_amount + ?", totalAmount),
		}).Error; err != nil {

		tx.Rollback()
		helpers.ErrorResponse(c, 500, "Update shift gagal", err)
		return
	}

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Transaction saved", gin.H{
        "id": tr.ID,
        "shift_id": shift.ID,
        "invoice": invoice,
        "kasir": user.Name,
        "customer_name": member.Name,
        "total_item": tr.TotalItem,
        "total_quantity": tr.TotalQuantity,
        "paid_amount": tr.PaidAmount,
        "change_amount": tr.ChangeAmount,
        "payment_method": tr.PaymentMethod,
        "subtotal": tr.Subtotal,
        "pembulatan": tr.RoundedPrice,
        "total_amount": tr.TotalAmount,
        "created_at": helpers.ToLocalTime(tr.CreatedAt, user.Store.Timezone),
        "store": gin.H{
            "name": user.Store.StoreName,
            "phone": user.Store.Phone,
            "address": user.Store.Address,
        },
        "ppn": gin.H{
            "tax": ppn.Ppn,
            "amount": ppnAmount,
        },
        "items": txRow,
    }))
}
func GetTransactionHistories(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    
    if page < 1 { page = 1 }
    offset := (page-1)*limit

    type txRow struct {
        ID uint64 `json:"id"`
        Invoice string `json:"invoice"`
        TotalItem int `json:"total_item"`
        TotalQuantity int `json:"total_quantity"`
        Kasir string `json:"kasir"`
        StoreName string `json:"store_name"`
        Subtotal float64 `json:"subtotal"`
        Tax float64 `json:"tax"`
        TotalAmount float64 `json:"total_amount"`
        Status string `json:"status"`
        PaymentMethod string `json:"payment_method"`
        CreatedAt time.Time `json:"created_at"`
    }

    var rows []txRow

    now, err := helpers.GetCurentTime(user.Store.Timezone)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
    // awal hari ini (00:00:00)
    startDate := time.Date(
        now.Year(), now.Month(), now.Day(),
        0, 0, 0, 0,
        now.Location(),
    )
    // akhir hari ini (23:59:59)
    endDate := startDate.Add(24*time.Hour - time.Nanosecond)
    startDate = startDate.UTC()
    endDate = endDate.UTC()

    baseWhere := "WHERE t.created_at >= ? AND t.created_at <= ?"
    args := []interface{}{startDate, endDate}
    if q != "" {
        like := "%"+q+"%"
        baseWhere += " AND (t.invoice LIKE ? OR u.name LIKE ? OR s.store_name LIKE ?)"
        args = append(args, like, like, like)
    }
    
    // count
    var total int64
    countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM transactions t LEFT JOIN users u ON u.id = t.user_id LEFT JOIN store_profiles s ON s.id = t.store_id %s`, baseWhere)
    if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to count transaction", err)
        return
    }

    dataSQL := fmt.Sprintf(`
        SELECT 
            t.id, 
            t.invoice,
            t.total_item,
            t.total_quantity,
            COALESCE(u.name, 'Unknown') AS kasir,
            COALESCE(s.store_name, '') AS store_name,
            t.subtotal,
            COALESCE(t.subtotal * t.tax / 100, 0) AS tax,
            t.total_amount,
            t.status, 
            t.payment_method, 
            t.created_at 
        FROM transactions t 
        LEFT JOIN users u ON u.id = t.user_id 
        LEFT JOIN store_profiles s ON s.id = t.store_id 
        %s 
        ORDER BY t.created_at DESC LIMIT ? OFFSET ?`, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil { helpers.ErrorResponse(c, 500, "Failed to fetch transactions", err); return }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    for i := range rows {
        rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, user.Store.Timezone)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "List riwayat transaksi",
        "resource": gin.H{
            "data": rows, 
            "pagination": pagination,
        },
    })
}
func DetailTransaction(c *gin.Context) {
    // user := c.MustGet("auth_user").(models.User)
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil {
        helpers.ErrorResponse(c, 400, "Invalid transaction id", err)
        return
    }

    type transactionItemResponse struct {
        ID       uint64  `json:"id"`
        Barcode  string  `json:"barcode"`
        ProductName     string  `json:"product_name"`
        Price    float64 `json:"price"`
        Quantity int64   `json:"quantity"`
    }

    // 🔹 Struct response
    var result struct {
        ID        uint64  `json:"id"`
        Invoice   string  `json:"invoice"`
        Kasir     string  `json:"kasir"`
        CustomerName string `json:"customer_name"`       
        TotalItem    int     `json:"total_item"`
        TotalQuantity int     `json:"total_quantity"`
        PaidAmount float64 `json:"paid_amount"`
        ChangeAmount float64 `json:"change_amount"`
        PaymentMethod string  `json:"payment_method"`
        Subtotal  float64 `json:"subtotal"`
        RoundedPrice  float64 `json:"pembulatan"`
        TotalAmount  float64 `json:"total_amount"`
        Status string `json:"status"`
        CreatedAt time.Time  `json:"created_at"`
        Store struct {
            Name string `json:"name"`
            Phone string `json:"phone"`
            Address string `json:"address"`
        } `json:"store"`

        Ppn struct {
            Tax   float64 `json:"tax"`
            Amount float64 `json:"amount"`
        } `json:"ppn"`

        Items []transactionItemResponse `json:"items"`
    }

    var tx models.Transaction
    if err := config.DB.
        Preload("User").
        Preload("Member", func(db *gorm.DB) *gorm.DB {
            return db.Unscoped()
        }).
        Preload("Store").First(&tx, id).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }

    tx.ToLocal(tx.Store.Timezone)

    result.ID = tx.ID
    result.Invoice = tx.Invoice
    result.Kasir = tx.User.Name
    result.CustomerName = tx.Member.Name
    result.TotalItem = tx.TotalItem
    result.TotalQuantity = tx.TotalQuantity
    result.PaidAmount = tx.PaidAmount
    result.ChangeAmount = tx.ChangeAmount
    result.PaymentMethod = tx.PaymentMethod
    result.Subtotal = tx.Subtotal
    result.TotalAmount = tx.TotalAmount
    result.RoundedPrice = tx.RoundedPrice
    result.Status = tx.Status
    result.CreatedAt = tx.CreatedAt
    
    result.Store.Name = tx.Store.StoreName
    result.Store.Phone = tx.Store.Phone
    result.Store.Address = tx.Store.Address
    
    result.Ppn.Tax = tx.Tax
    result.Ppn.Amount = tx.TaxPrice

    // ambil items
    var items []transactionItemResponse
    queryItems := `
        SELECT 
            ti.id,
            p.barcode,
            ti.product_name,
            ti.price,
            ti.quantity
        FROM transaction_items ti
        LEFT JOIN products p ON p.id = ti.product_id
        WHERE ti.transaction_id = ?
    `

    if err := config.DB.Raw(queryItems, id).Scan(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed get items", err)
        return
    }

    result.Items = items

    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "message":  "Detail transaksi",
        "resource": result,
    })
}
func DetailTransactionsShift(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)
    shiftIDParam := c.Param("shift_id")
    shiftID, err := strconv.ParseUint(shiftIDParam, 10, 64)
    if err != nil {
        helpers.ErrorResponse(c, 400, "Invalid shift id", err)
        return
    }

    // q := strings.TrimSpace(c.DefaultQuery("q", ""))
    // page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    // limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    
    // if page < 1 { page = 1 }
    // offset := (page-1)*limit

    var shift models.Shift
    if err := config.DB.
        Preload("Store").
        Preload("UserOpen").
        Preload("UserClosed").
        First(&shift, shiftID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Shift not found", err)
        return
    }

    timezone := "Asia/Jakarta"
    if user.Role == "kasir" {
        timezone = user.Store.Timezone
    }

    shift.ToLocal(timezone)

    var summary struct {
        TotalInvoice int64
        TotalRounded float64
        TotalAmount float64
        CashCancelled float64
		TransferCancelled float64
		QrisCancelled float64
    }

    summarySQL := `
        SELECT 
            COUNT(*) as total_invoice,
            COALESCE(SUM(CASE WHEN status = 'done' THEN rounded_price ELSE 0 END),0) AS total_rounded,
            COALESCE(SUM(CASE WHEN status = 'done' THEN total_amount ELSE 0 END),0) AS total_amount,
            COALESCE(SUM(CASE WHEN payment_method = 'cash' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS cash_cancelled,
			COALESCE(SUM(CASE WHEN payment_method = 'transfer' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS transfer_cancelled,
			COALESCE(SUM(CASE WHEN payment_method = 'qris' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS qris_cancelled
        FROM transactions t
        WHERE t.shift_id = ?
    `

    if err := config.DB.Raw(summarySQL, shiftID).Scan(&summary).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed summary", err)
        return
    }

    type txItemRow struct {
        ID uint64 `json:"id"`
        Status string `json:"status"`
        TransactionID uint64 `json:"transaction_id"`
        Invoice string `json:"invoice"`
        ProductName string `json:"product_name"`
        Quantity int `json:"quantity"`
        Price float64 `json:"price"`
        DiscountPrice float64 `json:"discount_price"`
        Subtotal float64 `json:"subtotal"`
        CreatedAt time.Time `json:"created_at"`
    }

    var result struct { 
        Start           time.Time  `json:"start"`
        End             *time.Time `json:"end"` // pointer kalau bisa null
        UserOpen        string     `json:"user_open"`
        UserClosed      string    `json:"user_closed"` // pointer kalau bisa null
        
        InitialCash     float64    `json:"initial_cash"`
        TotalInvoice    int64      `json:"total_invoice"`
        
        TotalCash       float64     `json:"total_cash"`
        TotalTransfer       float64     `json:"total_transfer"`
        TotalQris       float64     `json:"total_qris"`
        TotalCashCancel       float64     `json:"total_cash_cancel"`
        TotalTransferCancel       float64     `json:"total_transfer_cancel"`
        TotalQrisCancel       float64     `json:"total_qris_cancel"`
        
        TotalTax        float64    `json:"total_tax"`
        TotalSubtotal   float64    `json:"total_subtotal"`
        TotalAmount  float64    `json:"total_penjualan"`
        TotalRounded  float64    `json:"pembulatan"`
        ExpectedCash    float64    `json:"expected_cash"`
        ExpectedAmount    float64    `json:"expected_amount"`
        ActualCash      float64    `json:"actual_cash"`
        ActualAmount      float64    `json:"actual_amount"`
        Difference      float64    `json:"difference"`
        Note            *string    `json:"note"` // optional
        
        Store struct {
            Name  string  `json:"name"`
            Phone string `json:"phone"`
            Address string `json:"address"`
        } `json:"store"`

        Items []txItemRow `json:"items"`
    }

    var rows []txItemRow

    baseWhere := "WHERE t.shift_id = ?"
    args := []interface{}{shiftID}
    // if q != "" {
    //     like := "%"+q+"%"
    //     baseWhere += " AND (t.invoice LIKE ? OR ti.product_name LIKE ?)"
    //     args = append(args, like, like)
    // }

    // var total int64
    // countSQL := fmt.Sprintf(`
    //     SELECT COUNT(*) 
    //     FROM transaction_items ti
    //     JOIN transactions t ON t.id = ti.transaction_id
    //     %s
    // `, baseWhere)
    // if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
    //     helpers.ErrorResponse(c, 500, "Failed to count transaction items", err);
    //     return
    // }

    dataSQL := fmt.Sprintf(`
        SELECT 
            ti.id,
            t.status,
            ti.transaction_id,
            t.invoice,
            ti.product_name,
            ti.quantity,
            ti.price,
            ti.discount_price,
            ti.subtotal,
            ti.created_at
        FROM transaction_items ti
        JOIN transactions t ON t.id = ti.transaction_id
        %s
        ORDER BY ti.created_at DESC
    `, baseWhere)
    // args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Failed to fetch tx", err); 
        return 
    }

    for i := range rows {
        rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, timezone)
    }

    // lastPage := int(math.Ceil(float64(total)/float64(limit)))
    // pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    user_closed := "-"
    if shift.UserClosed != nil {
        user_closed = shift.UserClosed.Name
    }

    result.Start = shift.StartTime
    result.End = shift.EndTime
    result.UserOpen = shift.UserOpen.Name
    result.UserClosed = user_closed

    result.InitialCash = shift.InitialCash
    result.TotalInvoice = summary.TotalInvoice
    
    result.TotalCash = shift.TotalCash
    result.TotalTransfer = shift.TotalTransfer
    result.TotalQris = shift.TotalQris
    result.TotalCashCancel = summary.CashCancelled
    result.TotalTransferCancel = summary.TransferCancelled
    result.TotalQrisCancel = summary.QrisCancelled
    
    result.TotalTax = shift.TotalTax
    result.TotalSubtotal = shift.Subtotal
    result.TotalAmount = summary.TotalAmount
    result.TotalRounded = summary.TotalRounded
    result.ExpectedCash = shift.TotalCash + shift.InitialCash
    result.ExpectedAmount = shift.ExpectedAmount
    result.ActualCash = shift.ActualCash
    result.ActualAmount = shift.ExpectedAmount + shift.Difference
    result.Difference = shift.Difference
    result.Note = shift.Note
    
    result.Store.Name = shift.Store.StoreName
    result.Store.Phone = shift.Store.Phone
    result.Store.Address = shift.Store.Address
    result.Items = rows

    c.JSON(http.StatusOK, response.Success("Detail Transaction Shift", result))
}
func CancelTransaction(c *gin.Context) {
    txIDParam := c.Param("id")
    txID, err := strconv.ParseUint(txIDParam, 10, 64)
    if err != nil { 
        helpers.ErrorResponse(c, 400, "Invalid transaction id", err); 
        return 
    }

    var tr models.Transaction
    if err := config.DB.Preload("Items").First(&tr, txID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }

    if tr.Status != "done" {
        helpers.ErrorResponse(c, 422, "Only done transactions can be cancelled", nil)
        return
    }

    // check shift
    var shift models.Shift
    if err := config.DB.First(&shift, tr.ShiftID).Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Failed to load shift", err); 
        return 
    }
    if shift.Status != "open" { 
        helpers.ErrorResponse(c, 422, "Shift already closed; cannot cancel transaction", nil); 
        return 
    }

    // rollback transaction: mark transaction cancelled, restore product statuses, etc.
    dbTx := config.DB.WithContext(c.Request.Context()).Begin()
    if err := dbTx.Model(&tr).Update("status", "cancelled").Error; err != nil { 
        dbTx.Rollback(); 
        helpers.ErrorResponse(c, 500, "Failed to cancel tx", err); 
        return 
    }

    // restore products
    for _, item := range tr.Items {
        if err := dbTx.Model(&models.Product{}).Where("id = ?", item.ProductID).Update("status", "display").Error; err != nil { 
            dbTx.Rollback(); 
            helpers.ErrorResponse(c, 500, "Failed to restore product", err); 
            return 
        }
    }

    // INCREMENTAL SHIFT UPDATE
    var cash, transfer, qris float64
	switch tr.PaymentMethod {
	case "cash":
		cash = tr.TotalAmount
	case "transfer":
		transfer = tr.TotalAmount
	case "qris":
		qris = tr.TotalAmount
	}

	if err := dbTx.Model(&models.Shift{}).
		Where("id = ?", shift.ID).
		Updates(map[string]interface{}{
			"total_cash":     gorm.Expr("total_cash - ?", cash),
			"total_transfer": gorm.Expr("total_transfer - ?", transfer),
			"total_qris":     gorm.Expr("total_qris - ?", qris),
			"total_tax":      gorm.Expr("total_tax - ?", tr.TaxPrice),
			"subtotal":       gorm.Expr("subtotal - ?", tr.Subtotal),
			"expected_amount": gorm.Expr("expected_amount - ?", tr.TotalAmount),
		}).Error; err != nil {

		dbTx.Rollback()
		helpers.ErrorResponse(c, 500, "Update shift gagal", err)
		return
	}

    if err := dbTx.Commit().Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Commit failed", err); 
        return 
    }

    c.JSON(http.StatusOK, response.Success("Transaction cancelled", tr))
}
//ADMIN
func GetAllTransactions(c *gin.Context) {
    store_id := strings.TrimSpace(c.DefaultQuery("store_id", ""))
    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    if page < 1 { page = 1 }
    offset := (page-1)*limit

    var store models.StoreProfile

    type txRow struct {
        ID uint64 `json:"id"`
        Invoice string `json:"invoice"`
        TotalItem int `json:"total_item"`
        TotalQuantity int `json:"total_quantity"`
        CustomerName string `json:"customer_name"`
        // Kasir string `json:"kasir"`
        StoreName string `json:"store_name"`
        Subtotal float64 `json:"subtotal"`
        Tax float64 `json:"tax"`
        TotalAmount float64 `json:"total_amount"`
        Status string `json:"status"`
        PaymentMethod string `json:"payment_method"`
        CreatedAt time.Time `json:"created_at"`
    }

    var rows []txRow

    baseWhere := "WHERE 1=1"
    args := []interface{}{}

    if store_id != "" {
        storeID, _ := strconv.Atoi(store_id)
        if err := config.DB.First(&store, storeID).Error; err != nil {
            helpers.ErrorResponse(c, 404, "store not found", err)
            return
        }

        baseWhere += " AND t.store_id = ?"
        args = append(args, store.ID)
    }

    if q != "" {
        like := "%"+q+"%"
        baseWhere += " AND (t.invoice LIKE ? OR m.name LIKE ? OR s.store_name LIKE ?)"
        args = append(args, like, like, like, like)
    }

    var total int64
    countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM transactions t LEFT JOIN users u ON u.id = t.user_id LEFT JOIN members m ON m.id = t.member_id LEFT JOIN store_profiles s ON s.id = t.store_id LEFT JOIN shifts sh ON sh.id = t.shift_id %s`, baseWhere)
    if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to count transactions", err)
        return
    }

    dataSQL := fmt.Sprintf(`
        SELECT
            t.id,
            t.invoice,
            t.total_item,
            t.total_quantity,
            COALESCE(m.name, '') AS customer_name,
            COALESCE(s.store_name, '') AS store_name,
            t.subtotal,
            COALESCE(t.tax_price, 0) AS tax,
            t.total_amount,
            t.status,
            t.payment_method,
            t.created_at
        FROM transactions t
        LEFT JOIN members m ON m.id = t.member_id
        LEFT JOIN store_profiles s ON s.id = t.store_id
        %s
        ORDER BY t.created_at DESC
        LIMIT ? OFFSET ?`, baseWhere)

    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to fetch transactions", err)
        return
    }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    for i := range rows {
        rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, "Asia/Jakarta")
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "List semua transaksi",
        "resource": gin.H{
            "data": rows,
            "pagination": pagination,
        },
    })
}
