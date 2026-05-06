package controllers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
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
        ReferenceID   string `json:"reference_id" binding:"required"`
        Type string `json:"type" binding:"required,oneof=product packaging"` // product | packaging
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
			case "ReferenceID":
				if e.Tag() == "required" {
					errorsMap["reference_id"] = "Reference ID wajib diisi"
				}
			case "Type":
				if e.Tag() == "required" {
					errorsMap["type"] = "Type wajib diisi"
				}else {
                    errorsMap["type"] = "Type harus 'product' atau 'packaging'"
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

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    // cek cart aktif
	var cart models.Cart
	err := tx.Where("store_id = ? AND user_id = ? AND keep_code IS NULL", storeID, user.ID).
		First(&cart).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// buat cart baru
			cart = models.Cart{
				StoreID: storeID,
				UserID:  uint64(user.ID),
			}
			if err := tx.Create(&cart).Error; err != nil {
				tx.Rollback()
				helpers.ErrorResponse(c, 500, "Failed to create cart", err)
				return
			}
		} else {
			tx.Rollback()
			helpers.ErrorResponse(c, 500, "Failed to check cart", err)
			return
		}
	}

    var cartItem models.CartItem
    if p.Type == "product" {
        var product models.Product
		if err := tx.Where("barcode = ? AND store_id = ? AND deleted_at IS NULL", p.ReferenceID, storeID).
			First(&product).Error; err != nil {
			tx.Rollback()
			helpers.ErrorResponse(c, 404, "Product tidak ditemukan", err)
			return
		}
		if product.Status == "sale" {
			tx.Rollback()
			helpers.ErrorResponse(c, 422, "Barang sudah discan", nil)
			return
		}
        // update product status
        if err := tx.Model(&product).Update("status", "sale").Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to update product", err)
            return
        }

        cartItem = models.CartItem{
			CartID:      cart.ID,
			ProductID: &product.ID,
			Type:    "product",
			ProductName: product.Name,
			Quantity:    1,
			Price:       product.Price,
			Subtotal:    product.Price,
		}
        // insert cart item
        if err := tx.Create(&cartItem).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to create cart item", err)
            return
        }
    }else {
        // convert string -> uint64
        packagingID, errCek := strconv.ParseUint(p.ReferenceID, 10, 64)
        if errCek != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 400, "Packaging ID harus berupa angka", err)
            return
        }
        
        var packaging models.Packaging
        if err := tx.Where("id = ?", packagingID).
            First(&packaging).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 404, "Packaging tidak ditemukan", err)
            return
        }

        // cek apakah sudah ada di cart
        var existing models.CartItem
        err := tx.Where("cart_id = ? AND packaging_id = ?", cart.ID, packaging.ID).
            First(&existing).Error

        if err == nil {
            // sudah ada → update qty
            newQty := existing.Quantity + 1
            newSubtotal := float64(newQty) * existing.Price

            if err := tx.Model(&existing).Updates(map[string]interface{}{
                "quantity": newQty,
                "subtotal": newSubtotal,
            }).Error; err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 500, "Failed to update packaging", err)
                return
            }
            cartItem = existing
        } else if errors.Is(err, gorm.ErrRecordNotFound) {
            // belum ada → insert baru
            cartItem = models.CartItem{
                CartID:      cart.ID,
                PackagingID: &packaging.ID,
                Type:    "packaging",
                ProductName: packaging.Name,
                Quantity:    1,
                Price:       packaging.Price,
                Subtotal:    packaging.Price,
            }

            if err := tx.Create(&cartItem).Error; err != nil {
                tx.Rollback()
                helpers.ErrorResponse(c, 500, "Failed to create packaging item", err)
                return
            }
        } else {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to check packaging item", err)
            return
        }
    }

    //recalculate cart total
    if err := helpers.RecalculateCart(tx, cart.ID); err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to recalculate cart", err)
        return
    }

    if err := tx.Commit().Error; err != nil {
        helpers.ErrorResponse(c, 500, "Commit failed", err)
        return
    }

    c.JSON(http.StatusOK, response.Success("Added to cart", cartItem))
}
func RemoveItemCart(c *gin.Context) {
    // user := c.MustGet("auth_user").(models.User)
    cartID := c.Param("cart_item_id")

    var cartItem models.CartItem
    if err := config.DB.Where("id = ?", cartID).First(&cartItem).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Cart item not found", err)
        return
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if cartItem.Type == "product" {        
        //update data product back to display
        if err := tx.Model(&models.Product{}).Where("id = ?", *cartItem.ProductID).Update("status", "display").Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to update product status", err)
            return
        }
    }
    // delete cart item
    if err := tx.Delete(&cartItem).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to remove cart item", err)
        return
    }

    //recalculate cart total
    if err := helpers.RecalculateCart(tx, cartItem.CartID); err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to recalculate cart", err)
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
    if err := config.DB.Model(&models.Cart{}).
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

	if user.StoreID == nil {
		helpers.ErrorResponse(c, 403, "User does not have store ID", nil)
		return
	}
	storeID := *user.StoreID

	q := strings.TrimSpace(c.DefaultQuery("q", ""))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	type pendingGroup struct {
		CustomerName string  `json:"customer_name"`
		KeepCode     string  `json:"keep_code"`
		ItemCount    int64   `json:"item_count"`
		Total        float64 `json:"total"`
	}

	var groups []pendingGroup

	// base where dari carts
	baseWhere := "WHERE c.user_id = ? AND c.store_id = ? AND c.keep_code IS NOT NULL AND c.keep_code != ''"
	args := []interface{}{user.ID, storeID}

	if q != "" {
		baseWhere += " AND m.name LIKE ?"
		args = append(args, "%"+q+"%")
	}

	// count dari carts (bukan cart_items lagi)
	var total int64
	countSQL := `
		SELECT COUNT(*)
		FROM carts c
		LEFT JOIN members m ON m.id = c.member_id
	` + baseWhere

	if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to count pending groups", err)
		return
	}

	// ambil data
	dataSQL := fmt.Sprintf(`
		SELECT 
			COALESCE(m.name, '-') AS customer_name,
			c.keep_code,
			COUNT(ci.id) AS item_count,
			c.grand_total AS total
		FROM carts c
		LEFT JOIN cart_items ci ON ci.cart_id = c.id
		LEFT JOIN members m ON m.id = c.member_id
		%s
		GROUP BY c.id, c.keep_code, m.name
		ORDER BY c.updated_at DESC
		LIMIT ? OFFSET ?
	`, baseWhere)

	args = append(args, limit, offset)

	if err := config.DB.Raw(dataSQL, args...).Scan(&groups).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to list pending groups", err)
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(groups), int(total))

	c.JSON(http.StatusOK, response.Success("List Pending transactions", gin.H{
		"data":       groups,
		"pagination": pagination,
	}))
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
	var existing models.Cart
    if err := config.DB.Where("keep_code = ? AND store_id = ?", keep, storeID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 404, "Tidak ada cart yang ditemukan", nil)
		}else {
			helpers.ErrorResponse(c, 500, "Internal server error", err)
		}
		return
	}

    // check current active cart (without keep_code)
    var count int64
    if err := config.DB.Model(&models.Cart{}).
        Where("user_id = ? AND store_id = ? AND (keep_code = '' OR keep_code IS NULL)", user.ID, storeID).
        Count(&count).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to check cart", err)
        return
    }

    if count > 0 {
        helpers.ErrorResponse(c, 422, "Kosongkan/Pending cart terlebih dahulu", nil)
        return
    }

	//update cart
	if err := config.DB.Model(&models.Cart{}).Where("keep_code = ?", keep).Update("keep_code", nil).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Internal server error", err)
		return
	}

	var cart_items []models.CartItem
	if err := config.DB.Where("cart_id = ?", existing.ID).Find(&cart_items).Error; err != nil {
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
        ProductID     uint64  `json:"product_id"`
        Barcode       string  `json:"barcode"`
        ProductName   string  `json:"product_name"`
        Quantity      uint64   `json:"quantity"`
        Price         float64 `json:"price"`
        DiscountPrice float64 `json:"discount_price"`
        Subtotal      float64 `json:"subtotal"`
    }

    //load ppn
    var ppn models.Ppn
    if err := config.DB.Where("is_tax_default = ?", true).First(&ppn).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            helpers.ErrorResponse(c, 404, "PPN tidak ditemukan", nil)
        } else {
            helpers.ErrorResponse(c, 500, "Internal server error", err)
        }
        return     
    }

    //load cart 
    var cart models.Cart
    if err := config.DB.Where("user_id = ? AND store_id = ? AND (keep_code IS NULL OR keep_code = '')", user.ID, storeID).First(&cart).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            payload := gin.H{
                "products": nil,
                "items": nil,
                "items_packaging": nil,
                "subtotal": 0,
                "ppn": gin.H{
                    "tax": ppn.Ppn,
                    "amount": 0,
                },
                "total_amount": 0,
                "pembulatan": 0,
            }

            c.JSON(http.StatusOK, response.Success("Current cart", payload))
        }else {
            helpers.ErrorResponse(c, 500, "Internal server error", err)
        }
        return
    }

    var cartItems []models.CartItem
    var items []cartItemResponse
    err := config.DB.Model(&models.CartItem{}).
        Preload("Product").Preload("Packaging").
        Where("cart_id = ?", cart.ID).
        Find(&cartItems).Error

    if err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load current cart", err)
        return
    }

    type txRowPriceModel struct {
        Name string `json:"name"`
        Price     float64  `json:"price"`
        Quantity    int64 `json:"quantity"`
        Total    float64 `json:"total"`
    }
    type txPackagingModel struct {
        ID uint64 `json:"id"`
        Name string `json:"name"`
        Price     float64  `json:"price"`
        Quantity    uint64 `json:"quantity"`
        Total    float64 `json:"total"`
    }

    //item packaging
    var itemsPackaging []txPackagingModel
    priceMap := make(map[float64]int64)
    for _, it := range cartItems {
        
        if it.Type == "product" && it.Product != nil {
            priceMap[it.Price] += 1
            items = append(items, cartItemResponse{
                ID: it.ID,
                ProductID: *it.ProductID,
                Barcode: it.Product.Barcode,
                ProductName: it.Product.Name,
                Quantity: it.Quantity,
                Price: it.Price,
                DiscountPrice: it.DiscountPrice,
                Subtotal: it.Subtotal,
            })
        }else if it.Type == "packaging" && it.Packaging != nil {
            itemsPackaging = append(itemsPackaging, txPackagingModel{
                ID: it.ID,
                Name: it.Packaging.Name,
                Price: it.Price,
                Quantity: it.Quantity,
                Total: it.Price * float64(it.Quantity),
            })
        }else {
            helpers.ErrorResponse(c, 500, "Unknown cart item type", fmt.Errorf("unknown cart item type: %s", it.Type))
            return
        }
    }

    //grouping item berdasarkan harga (untuk kebutuhan struk nanti)
    var itemsPrice []txRowPriceModel
    for price, qty := range priceMap {
        itemsPrice = append(itemsPrice, txRowPriceModel{
            Name: formatPriceToProductName(price),
            Price:    price,
            Quantity: qty,
            Total: price * float64(qty),
        })
    }
    // sorting dari harga terendah
    sort.Slice(itemsPrice, func(i, j int) bool {
        return itemsPrice[i].Price < itemsPrice[j].Price
    })

    ppn_price := math.Round(cart.GrandTotal * (ppn.Ppn / 100))
    totalBelanja := math.Round(cart.GrandTotal + ppn_price)
    totalAmount := helpers.RoundTo500(int(totalBelanja))
    pembulatan := totalAmount - totalBelanja

    payload := gin.H{
        "products": items,
        "items": itemsPrice,
        "items_packaging": itemsPackaging,
        "subtotal": cart.GrandTotal,
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
    
    // load cart
    var cart models.Cart
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code = ?", user.ID, storeID, keep).First(&cart).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Cart not found for given keep_code", err)
        return
    }

    // load cart items to know affected products
    var items []models.CartItem
    if err := config.DB.Where("cart_id = ?", cart.ID).Find(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart items", err)
        return
    }

    // collect product ids
    prodIDs := make([]uint64, 0, len(items))
    if len(items) > 0 {        
        for _, it := range items {
            if it.Type == "product" || it.ProductID != nil {
                prodIDs = append(prodIDs, *it.ProductID)
            }
        }
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    if err := tx.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart items", err)
        return
    }
    //delete cart
    if err := tx.Delete(&cart).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart", err)
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

    c.JSON(http.StatusOK, response.Success("Cart pending removed", nil))
}
func EmptyCurrentCart(c *gin.Context) {
    user := c.MustGet("auth_user").(models.User)

    // load cart
    var cart models.Cart
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, *user.StoreID).Find(&cart).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart", err)
        return
    }
    var items []models.CartItem
    if err := config.DB.Where("cart_id = ?", cart.ID).Find(&items).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart items", err)
        return
    }

    // collect product ids
    prodIDs := make([]uint64, 0, len(items))
    if len(items) > 0 {
        for _, it := range items {
            if it.Type == "product" || it.ProductID != nil {
                prodIDs = append(prodIDs, *it.ProductID)
            }
        }
    }

    tx := config.DB.WithContext(c.Request.Context()).Begin()
    // delete cart items
    if err := tx.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart items", err)
        return
    }
    // delete cart
    if err := tx.Delete(&cart).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart", err)
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

    // load cart
    var cart models.Cart
    if err := config.DB.Where("user_id = ? AND store_id = ? AND keep_code IS NULL", user.ID, storeID).Find(&cart).Error; err != nil {
        helpers.ErrorResponse(c, 500, "Failed to load cart", err)
        return
    }
    // load cart items
    var items []models.CartItem
    if err := config.DB.Where("cart_id = ?", cart.ID).Find(&items).Error; err != nil {
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
    totalPackagingQty := uint64(0)
    totalPackagingPrice := float64(0)
	subTotal := float64(0)
    
    type txRowPriceModel struct {
        Name string `json:"name"`
        Price     float64  `json:"price"`
        Quantity    uint64 `json:"quantity"`
        Total    float64 `json:"total"`
    }
    priceMap := make(map[float64]uint64)
    var productIDs []uint64
    var itemsPackaging []txRowPriceModel
    // migrate items
    for _, it := range items {
        ti := models.TransactionItem{
            StoreID: storeID,
            TransactionID: tr.ID,
            ProductName: it.ProductName,
            Quantity: it.Quantity,
            Price: it.Price,
            DiscountPrice: it.DiscountPrice,
            Subtotal: it.Subtotal,
        }

        if it.Type == "product" {
            ti.ProductID = it.ProductID
            ti.PackagingID = nil
            productIDs = append(productIDs, *it.ProductID)
            priceMap[it.Price] += 1
        }else {
            totalPackagingQty += it.Quantity
            totalPackagingPrice += it.Price * float64(it.Quantity) 

            ti.PackagingID = it.PackagingID
            ti.ProductID = nil

            itemsPackaging = append(itemsPackaging, txRowPriceModel{
                Name: it.ProductName,
                Price: it.Price,
                Quantity: it.Quantity,
                Total: it.Price * float64(it.Quantity),
            })
        }

        if err := tx.Create(&ti).Error; err != nil {
            tx.Rollback()
            helpers.ErrorResponse(c, 500, "Failed to create transaction item", err)
            return
        }

        totalQty += int(it.Quantity)
		subTotal += it.Subtotal
    }

    //Grouping price
    var txRowPrice []txRowPriceModel
    for price, qty := range priceMap {
        txRowPrice = append(txRowPrice, txRowPriceModel{
            Name: formatPriceToProductName(price),
            Price:    price,
            Quantity: qty,
            Total: price * float64(qty),
        })
    }
    // sorting dari harga terendah
    sort.Slice(txRowPrice, func(i, j int) bool {
        return txRowPrice[i].Price < txRowPrice[j].Price
    })
    //tambahkan packaging list
    txRowPrice = append(txRowPrice, itemsPackaging...)

    // hitung ppn dan total
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
        "total_packaging_qty": totalPackagingQty,
        "total_packaging_price": totalPackagingPrice,
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
    if err := tx.Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart item", err)
        return
    }
    // delete cart
    if err := tx.Delete(&cart).Error; err != nil {
        tx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to delete cart", err)
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
        "items": txRowPrice,
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

    type transactionPerItem struct {
        ID       uint64  `json:"id"`
        Barcode  string  `json:"barcode"`
        ProductName     string  `json:"product_name"`
        Price    float64 `json:"price"`
        Quantity uint64   `json:"quantity"`
    }
    
    type groupingItemPrice struct {
        Name string `json:"name"`
        Price    float64 `json:"price"`
        Quantity uint64   `json:"quantity"`
        Total float64   `json:"total"`
    }

    // 🔹 Struct response
    var result struct {
        ID        uint64  `json:"id"`
        Invoice   string  `json:"invoice"`
        Kasir     string  `json:"kasir"`
        UserCancel     string  `json:"user_cancel"`
        CustomerName string `json:"customer_name"`       
        TotalItem    int     `json:"total_item"`
        TotalQuantity int     `json:"total_quantity"`
        TotalPackagingQty uint    `json:"total_packaging_qty"`
        TotalPackagingPrice float64 `json:"total_packaging_price"`
        PaidAmount float64 `json:"paid_amount"`
        ChangeAmount float64 `json:"change_amount"`
        PaymentMethod string  `json:"payment_method"`
        Subtotal  float64 `json:"subtotal"`
        RoundedPrice  float64 `json:"pembulatan"`
        TotalAmount  float64 `json:"total_amount"`
        Status string `json:"status"`
        CreatedAt time.Time  `json:"created_at"`
        Note string `json:"note"`
        Store struct {
            Name string `json:"name"`
            Phone string `json:"phone"`
            Address string `json:"address"`
        } `json:"store"`

        Ppn struct {
            Tax   float64 `json:"tax"`
            Amount float64 `json:"amount"`
        } `json:"ppn"`

        Products []transactionPerItem `json:"products"`
        Items []groupingItemPrice `json:"items"`
    }

    var tx models.Transaction
    if err := config.DB.
        Preload("User", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("UserCancel", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("Member", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("Items.Product").
		Preload("Items.Packaging").
        Preload("Store").First(&tx, id).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }

    tx.ToLocal(tx.Store.Timezone)

    userCancel := "-"
    if tx.UserCancel != nil {
        userCancel = tx.UserCancel.Name
    }

    result.ID = tx.ID
    result.Invoice = tx.Invoice
    result.Kasir = tx.User.Name
    result.UserCancel = userCancel
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
    result.Note = tx.Note
    
    result.Store.Name = tx.Store.StoreName
    result.Store.Phone = tx.Store.Phone
    result.Store.Address = tx.Store.Address
    
    result.Ppn.Tax = tx.Tax
    result.Ppn.Amount = tx.TaxPrice

    // ambil per items
    priceMap := make(map[float64]uint64)
    var itemsPackaging []groupingItemPrice
    for _, item := range tx.Items {
        if item.ProductID != nil && item.Product != nil {            
            result.Products = append(result.Products, transactionPerItem{
                ID: item.ID,
                Barcode: item.Product.Barcode,
                ProductName: item.ProductName,
                Price: item.Price,
                Quantity: item.Quantity,
            })

            // gourp item by price
            priceMap[item.Price] += item.Quantity
        }else if item.PackagingID != nil && item.Packaging != nil {
            itemsPackaging = append(itemsPackaging, groupingItemPrice{
                Name: item.ProductName,
                Price: item.Price,
                Quantity: item.Quantity,
                Total: item.Price * float64(item.Quantity),
            })
        }
    }
    // ambil item packaging
    for price, qty := range priceMap {
        result.Items = append(result.Items, groupingItemPrice{
            Name: formatPriceToProductName(price),
            Price:    price,
            Quantity: qty,
            Total: price * float64(qty),
        })
    }
    // sorting dari harga terendah
    sort.Slice(result.Items, func(i, j int) bool {
        return result.Items[i].Price < result.Items[j].Price
    })
    //tambahkkan packaging list
    result.Items = append(result.Items, itemsPackaging...)

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
        TotalPackagingQty uint
        TotalPackagingPrice float64
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

            COALESCE(SUM(CASE WHEN status = 'done' THEN total_packaging_qty ELSE 0 END),0) AS total_packaging_qty,
			COALESCE(SUM(CASE WHEN status = 'done' THEN total_packaging_price ELSE 0 END),0) AS total_packaging_price,

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
        TotalPackagingQty uint `json:"total_packaging_qty"`
        TotalPackagingPrice float64 `json:"total_packaging_price"`
        
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
    result.TotalPackagingQty = summary.TotalPackagingQty
    result.TotalPackagingPrice = summary.TotalPackagingPrice
    
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
    user := c.MustGet("auth_user").(models.User)
    txIDParam := c.Param("id")
    txID, err := strconv.ParseUint(txIDParam, 10, 64)
    if err != nil { 
        helpers.ErrorResponse(c, 400, "Invalid transaction id", err); 
        return 
    }

    type payload struct {
        Note string `json:"note" binding:"required"`
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
			case "Note":
				if e.Tag() == "required" {
					errorsMap["note"] = "Catatan pembatalan wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

// LOAD TRANSACTION
    var tr models.Transaction
    if err := config.DB.Preload("Items").First(&tr, txID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }
    if tr.Status != "done" {
        helpers.ErrorResponse(c, 422, "Only done transactions can be processed", nil)
        return
    }

// CHECK SHIFT
    var shift models.Shift
    if err := config.DB.First(&shift, tr.ShiftID).Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Failed to load shift", err); 
        return 
    }
    // jika user role kasir, pastikan shift masih open
    if user.Role == "kasir" {        
        if shift.Status != "open" { 
            helpers.ErrorResponse(c, 422, "Shift already closed; cannot cancel transaction", nil); 
            return 
        }
    }

//UPDATE TRANSACTION STATUS
    dbTx := config.DB.WithContext(c.Request.Context()).Begin()
    updates := map[string]interface{}{
        "status": "cancelled",
        "note": p.Note,
        "cancelled_by": user.ID,
    }
    // jika user kasir, set status pending_cancel untuk menunggu approval admin
    if user.Role == "kasir" {
        updates["status"] = "pending_cancel"
    }
    if err := dbTx.Model(&tr).Updates(updates).Error; err != nil { 
        dbTx.Rollback(); 
        helpers.ErrorResponse(c, 500, "Failed to cancel tx", err); 
        return 
    }

// JIKA BUKAN KASIR, MAKA LANGSUNG PROSES PEMBATALAN
    if user.Role != "kasir"{
        // restore products
        productIDs := make([]uint64, 0)
        for _, item := range tr.Items {
            if item.ProductID != nil {
                productIDs = append(productIDs, *item.ProductID)
            }
        }
        if err := dbTx.Model(&models.Product{}).Where("id IN ?", productIDs).Update("status", "display").Error; err != nil { 
            dbTx.Rollback(); 
            helpers.ErrorResponse(c, 500, "Failed to restore product status", err); 
            return 
        }
        //lakukan recalculate summary shift
        summary, err := helpers.RecalculateTransactionShift(dbTx, tr.StoreID, shift.ID)
        if err != nil {
            helpers.ErrorResponse(c, 422, "Recalculate summary transaction shift failed", err)
            return
        }

        ExpectedAmount := shift.InitialCash + summary["total_amount"].(float64)
        ExpectedCash := summary["cash"].(float64) + shift.InitialCash
        diff := shift.ActualCash - ExpectedCash

        updates := map[string]interface{}{
            "total_cash": summary["cash"].(float64),
            "total_transfer": summary["transfer"].(float64),
            "total_qris": summary["qris"].(float64),
            "total_tax": summary["tax_amount"].(float64),

            "subtotal": summary["subtotal"].(float64),
            "expected_amount": ExpectedAmount,
        }

        if shift.Status == "closed" {
            updates["difference"] = diff
        }

        if err := config.DB.Model(&shift).Updates(updates).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to update summary shift", err)
            return
        }
    }

// COMMIT TRANSACTION
    if err := dbTx.Commit().Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Commit failed", err); 
        return 
    }

    c.JSON(http.StatusOK, response.Success("Transaction cancellation request sent successfully", tr))
}
//ADMIN
func ApprovalCancelTransaction(c *gin.Context) {  
    txIDParam := c.Param("id")
    txID, err := strconv.ParseUint(txIDParam, 10, 64)
    if err != nil {
        helpers.ErrorResponse(c, 400, "Invalid transaction id", err)
        return
    }
    type payload struct {
        ApproveStatus string `json:"approve_status" binding:"required,oneof=approved rejected"`
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
			case "Note":
                errorsMap["approve_status"] = "Status approval wajib diisi dan harus bernilai 'approved' atau 'rejected'"
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

// LOAD TRANSACTION
    var tr models.Transaction
    if err := config.DB.Preload("Items").First(&tr, txID).Error; err != nil {
        helpers.ErrorResponse(c, 404, "Transaction not found", err)
        return
    }
    if tr.Status != "pending_cancel" {
        helpers.ErrorResponse(c, 422, "Only pending cancel transactions can be processed", nil)
        return
    }

// CHECK SHIFT
    var shift models.Shift
    if err := config.DB.First(&shift, tr.ShiftID).Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Failed to load shift", err); 
        return 
    }

// START TRANSACTION
    dbTx := config.DB.WithContext(c.Request.Context()).Begin()
    status := "done"

    // jika approved, maka update status transaction menjadi cancelled dan lakukan penyesuaian shift
    if p.ApproveStatus == "approved" {
        status = "cancelled"
        
        // restore products
        productIDs := make([]uint64, 0)
        for _, item := range tr.Items {
            if item.ProductID == nil {
                productIDs = append(productIDs, *item.ProductID)
            }
        }
        if err := dbTx.Model(&models.Product{}).Where("id IN ?", productIDs).Update("status", "display").Error; err != nil { 
            dbTx.Rollback(); 
            helpers.ErrorResponse(c, 500, "Failed to restore product status", err); 
            return 
        }
        //lakukan recalculate summary shift
        summary, err := helpers.RecalculateTransactionShift(dbTx, tr.StoreID, shift.ID)
        if err != nil {
            helpers.ErrorResponse(c, 422, "Recalculate summary transaction shift failed", err)
            return
        }

        ExpectedAmount := shift.InitialCash + summary["total_amount"].(float64)
        ExpectedCash := summary["cash"].(float64) + shift.InitialCash
        diff := shift.ActualCash - ExpectedCash

        updates := map[string]interface{}{
            "total_cash": summary["cash"].(float64),
            "total_transfer": summary["transfer"].(float64),
            "total_qris": summary["qris"].(float64),
            "total_tax": summary["tax_amount"].(float64),

            "subtotal": summary["subtotal"].(float64),
            "expected_amount": ExpectedAmount,
        }

        if shift.Status == "closed" {
            updates["difference"] = diff
        }

        if err := config.DB.Model(&shift).Updates(updates).Error; err != nil {
            helpers.ErrorResponse(c, 500, "Failed to update summary shift", err)
            return
        }
    }
    //update transaction status
    if err := dbTx.Model(&tr).Update("status", status).Error; err != nil {
        dbTx.Rollback()
        helpers.ErrorResponse(c, 500, "Failed to update transaction status", err)
        return
    }

// COMMIT TRANSACTION
    if err := dbTx.Commit().Error; err != nil { 
        helpers.ErrorResponse(c, 500, "Commit failed", err); 
        return 
    }

    c.JSON(http.StatusOK, response.Success("Transaction cancelled successfully", tr))
}
func GetPendingCancelTransactions(c *gin.Context) {
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

    baseWhere := "WHERE 1=1 AND t.status = 'pending_cancel'"
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

    baseWhere := "WHERE 1=1 AND t.status IN ('done', 'cancelled')"
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


//==================== Helper Function ====================
func formatPriceToProductName(price float64) string {
	// bulatkan dulu biar aman dari float
	p := int(math.Round(price))

	ribu := p / 1000
	sisa := p % 1000

	if ribu > 0 && sisa > 0 {
		return fmt.Sprintf("Produk %d Ribu %d Rupiah", ribu, sisa)
	}

	if ribu > 0 {
		return fmt.Sprintf("Produk %d Ribu", ribu)
	}

	return fmt.Sprintf("Produk %d Rupiah", sisa)
}