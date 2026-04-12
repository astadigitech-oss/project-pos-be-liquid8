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
        MemberID      uint64 `json:"member_id" binding:"required"`
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
			case "MemberID":
				if e.Tag() == "required" {
					errorsMap["member_id"] = "Member ID wajib diisi"
				}
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

    //cari member
    var member models.Member
    if err := config.DB.First(&member, p.MemberID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Member tidak ditemukan", err)
        return
    }

    // load product
    var product models.Product
    if err := config.DB.Where("barcode = ? AND store_id = ?", p.ProductBarcode, storeID).First(&product).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Product tidak ditemukan", err)
        return
    }

	if product.Status == "sale" {
		helpers.ErrorResponse(c, 422, "Product sudah terjual", nil)
		return
	}

    // create cart item
    cart := models.CartItem{
        StoreID: storeID,
        UserID:  uint64(user.ID),
        MemberID: member.ID,
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
    if err := tx.Model(&product).Updates(map[string]interface{}{"status": "sale"}).Error; err != nil {
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
    storeID := uint64(0)
    if user.StoreID != nil { 
		storeID = *user.StoreID 
	}else {
		helpers.ErrorResponse(c, 403, "User tidak memiliki store ID", nil)
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
        Updates(map[string]interface{}{"keep_code": keep}).Error; err != nil {
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

    type pendingGroup struct {
        CustomerName string  `json:"customer_name"`
        KeepCode     string  `json:"keep_code"`
        ItemCount    int64   `json:"item_count"`
        Total float64 `json:"total"`
    }

    var groups []pendingGroup

    // build raw query to aggregate
    baseSQL := `
		SELECT 
            m.name AS customer_name,
			ci.keep_code, 
			COUNT(*) AS item_count, 
			COALESCE(SUM(ci.subtotal),0) AS total
        FROM cart_items ci
        JOIN members m ON ci.member_id = m.id
        WHERE ci.user_id = ? 
			AND ci.store_id = ? 
			AND ci.keep_code IS NOT NULL 
			AND ci.keep_code != ''
		GROUP BY ci.keep_code ORDER BY ci.created_at DESC
	`

	if err := config.DB.Raw(baseSQL, user.ID, storeID).Scan(&groups).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to list pending groups", err)
		return
	}

    c.JSON(http.StatusOK, response.Success("List Pending transactions", groups))
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

    var items []models.CartItem
    if err := config.DB.Where("user_id = ? AND store_id = ? AND (keep_code IS NULL OR keep_code = '')", user.ID, storeID).Find(&items).Error; err != nil {
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

    var totalSubtotal float64
    var totalQuantity int64
    for _, it := range items {
        totalSubtotal += it.Subtotal
        totalQuantity += it.Quantity
    }

    ppn_price := totalSubtotal * (ppn.Ppn / 100)
    payload := gin.H{
        "items": items,
        "subtotal": totalSubtotal,
        "ppn": gin.H{
            "tax": ppn.Ppn,
            "amount": ppn_price,
        },
        "total_amount": totalSubtotal + ppn_price,
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

    storeID := uint64(0)
    if user.StoreID == nil {
		helpers.ErrorResponse(c, 400, "User does not have store ID", nil)
		return
	}
	storeID = *user.StoreID

    type payload struct {
		MemberID	uint64	`json:"member_id" binding:"required"`
        PaymentMethod string `json:"payment_method" binding:"required,oneof=cash transfer qris"`
        PaidAmount float64 `json:"paid_amount" binding:"required,gte=0"`
        Tax float64 `json:"tax" binding:"gte=0,max=100"`
        GrandTotal float64 `json:"grand_total" binding:"gte=0"`
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
			case "PaymentMethod":
				if e.Tag() == "required" {
					errorsMap["payment_method"] = "Payment method wajib diisi"
				}else {
					errorsMap["payment_method"] = "Payment method harus cash, transfer atau qris"
				}
			case "MemberID":
				if e.Tag() == "required" {
					errorsMap["member_id"] = "Member ID wajib diisi"
				}
			case "PaidAmount":
				if e.Tag() == "required" {
					errorsMap["paid_amount"] = "Paid amount wajib diisi"
				}
			case "Tax":
				if e.Tag() == "gte" {
					errorsMap["tax"] = "tax minimal 0"
				}
				if e.Tag() == "max" {
					errorsMap["tax"] = "tax maximal 100"
				}
			case "GrandTotal":
				if e.Tag() == "gte" {
					errorsMap["grand_total"] = "Grand total minimal 0"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

    // load member
    var member models.Member
    if err := config.DB.First(&member, p.MemberID).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "Member not found", nil)
        }else {
            helpers.ErrorResponse(c, 500, "Failed to load member", err)
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

    now := helpers.GetCurentTime()
    tr := models.Transaction{
        StoreID: storeID,
        UserID: uint64(user.ID),
        ShiftID: shift.ID,
        Invoice: invoice,
        MemberID: &p.MemberID,
        TotalItem: len(items),
        Tax: p.Tax,
        PaidAmount: p.PaidAmount,
        PaymentMethod: p.PaymentMethod,
        Status: "done",
        TransactionDate: now.Format("2006-01-02"),
    }

    if err := tx.Create(&tr).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to create transaction", err)
        return
    }

    totalQty := 0
	subTotal := float64(0)
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

        if err := tx.Create(&ti).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to create transaction item", err)
            return
        }

        totalQty += int(it.Quantity)
		subTotal += it.Subtotal
    }

    
	totalAmount := subTotal + (subTotal * (float64(p.Tax) / float64(100)))
	if totalAmount != p.GrandTotal {
		tx.Rollback()
        c.JSON(422, gin.H{
            "success": false,
            "message": "Total amount not match",
            "error": gin.H{
                "payload": p.GrandTotal,
                "expected_amount": totalAmount,
            },
        })
		return
	}

	changeAmount := tr.PaidAmount - totalAmount
	if changeAmount < 0 {
		tx.Rollback()
		helpers.ErrorResponse(c, 422, fmt.Sprintf("Paid amount (%.2f) tidak boleh kurang dari total amount (%.2f)", p.PaidAmount, totalAmount), nil)
		return
	}
    // update transaction totals
    if err := tx.Model(&tr).Updates(map[string]interface{}{
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

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Transaction saved", tr))
}
func GetTransactionHistories(c *gin.Context) {
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

    now := helpers.GetCurentTime()
    // awal hari ini (00:00:00)
    startDate := time.Date(
        now.Year(), now.Month(), now.Day(),
        0, 0, 0, 0,
        now.Location(),
    )
    // akhir hari ini (23:59:59)
    endDate := startDate.Add(24*time.Hour - time.Nanosecond)

    fmt.Print(startDate, endDate)
    baseWhere := "WHERE t.created_at >= ? AND t.created_at <= ?"
    args := []interface{}{startDate, endDate}
    if q != "" {
        like := "%"+q+"%"
        baseWhere += "AND (t.invoice LIKE ? OR u.name LIKE ? OR s.store_name LIKE ?)"
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
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil {
        helpers.ErrorResponse(c, 400, "Invalid transaction id", err)
        return
    }

    var tx models.Transaction
    if err := config.DB.Preload("User").
        Preload("Store").Preload("Items").
        First(&tx, id).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Detail transaksi",
        "resource": tx,
    })
}
func DetailTransactionsShift(c *gin.Context) {
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
        Preload("UserOpen").
        Preload("UserClosed").
        First(&shift, shiftID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Shift not found", err)
        return
    }

    var summary struct {
        TotalInvoice int64 `json:"total_invoice"`
        TotalSubtotal float64 `json:"total_subtotal"`
        TotalTax float64 `json:"total_tax"`
        TotalAmount float64 `json:"total_amount"`
    }

    summarySQL := `
        SELECT 
            COUNT(*) as total_invoice,
            COALESCE(SUM(t.subtotal), 0) as total_subtotal,
            COALESCE(SUM(t.subtotal * t.tax / 100), 0) as total_tax,
            COALESCE(SUM(t.total_amount), 0) as total_amount
        FROM transactions t
        WHERE t.shift_id = ? AND t.status = 'done'
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

    // lastPage := int(math.Ceil(float64(total)/float64(limit)))
    // pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    c.JSON(http.StatusOK, response.Success("Detail Transaction Shift", gin.H{
        "summary": gin.H{
            "start": shift.StartTime,
            "end": shift.EndTime,
            "user_open": shift.UserOpen.Name,
            "user_closed": shift.UserClosed.Name,
            "initial_cash": shift.InitialCash,
            "expected_cash": shift.ExpectedCash,
            "actual_cash": shift.ActualCash,
            "difference": shift.Difference,
            "note": shift.Note,
            "total_invoice": summary.TotalInvoice,
            "total_subtotal": summary.TotalSubtotal,
            "total_tax": summary.TotalTax,
            "total_penjualan": summary.TotalAmount,
        },
        "items": rows,
    }))
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

    if err := dbTx.Commit().Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Commit failed", err); 
        return 
    }

    c.JSON(http.StatusOK, response.Success("Transaction cancelled", tr))
}

