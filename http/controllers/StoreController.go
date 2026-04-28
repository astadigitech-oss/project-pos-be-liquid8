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
)

// ListStores returns paginated store profiles
func ListStores(c *gin.Context) {
	q := strings.TrimSpace(c.DefaultQuery("q", ""))
	// page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	// if page < 1 {
	// 	page = 1
	// }
	// limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	// offset := (page - 1) * limit

	type formatRes struct {
		ID              uint       `json:"id"`
		StoreName		string	   `json:"store_name"`
		Phone        	string     `json:"phone"`
		Address         string     `json:"address"`
		TotalProduct	int64		`json:"total_product"`
		TotalSales		float64		`json:"total_sales"`
	}

	var results []formatRes

	db := config.DB.Model(&models.StoreProfile{}).
		Select(`
			store_profiles.id,
			store_profiles.store_name,
			store_profiles.phone,
			store_profiles.address,
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
		db = db.Where("store_profiles.store_name LIKE ? OR store_profiles.phone LIKE ? OR store_profiles.address LIKE ?", like, like, like)
	}

	// count total
	// var total int64
	// if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
	// 	helpers.ErrorResponse(c, 500, "failed to count stores", err)
	// 	return
	// }

	// fetch data
	if err := db.Scan(&results).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to fetch stores", err)
		return
	}

	// lastPage := int(math.Ceil(float64(total) / float64(limit)))
	// pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(results), int(total))

	c.JSON(http.StatusOK, response.Success("List stores", results))
}
func DetailStore(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var store models.StoreProfile
	if err := config.DB.First(&store, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "store not found", err)
		return
	}

	// products pagination for this store
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
		Barcode string `json:"barcode"`
		Name string `json:"name"`
		Price float64 `json:"price"`
		TagColor string `json:"tag_color"`
		Quantity int64 `json:"quantity"`
		Status string `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}

	var rows []productRow

	baseWhere := "WHERE p.store_id = ? AND p.status = 'display'"
	args := []interface{}{id}
	if q != "" {
		like := "%" + q + "%"
		baseWhere += " AND (p.name LIKE ? OR p.barcode LIKE ?)"
		args = append(args, like, like)
	}

	// count
	var totalData int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM products p %s`, baseWhere)
	if err := config.DB.Raw(countSQL, args...).Scan(&totalData).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count products", err)
		return
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
			p.created_at
		FROM products p
		%s ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, baseWhere)

	args = append(args, limit, offset)
	if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to fetch products", err)
		return
	}

	for i := range rows {
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, store.Timezone)
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

	// total sales for this store (only completed/done transactions)
	var totalSales float64
	if err := config.DB.Raw("SELECT COALESCE(SUM(total_amount),0) FROM transactions WHERE store_id = ? AND status = ?", id, "done").Scan(&totalSales).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to calculate total sales", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("Detail store", gin.H{
		"store": store,
		"products": gin.H{"data": rows, "pagination": pagination},
		"total_sales": totalSales,
	}))
}
func StoreTransactionsHistories(c *gin.Context) {
	idParam := c.Param("id")
	storeID, _ := strconv.Atoi(idParam)

	var store models.StoreProfile
	if err := config.DB.First(&store, storeID).Error; err != nil {
		helpers.ErrorResponse(c, 404, "store not found", err)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 { page = 1 }
	offset := (page - 1) * limit

	// determine current week's Monday..Sunday in store timezone
	now, err := helpers.GetCurentTime("Asia/Jakarta")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
	// weekday := int(now.Weekday())
	// if weekday == 0 { weekday = 7 } // Sunday -> 7
	start := time.Date(now.Year(), now.Month(), now.Day()-7, 0, 0, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), now.Location())
	startUTC := start.UTC()
	endUTC := end.UTC()

	type txRow struct {
		ID uint64 `json:"id"`
		Invoice string `json:"invoice"`
		TotalAmount float64 `json:"total_amount"`
		TotalItem int `json:"total_item"`
		TotalQuantity int `json:"total_quantity"`
		PaymentMethod string `json:"payment_method"`
		Status string `json:"status"`
		UserName string `json:"user_name"`
		CreatedAt time.Time `json:"created_at"`
	}

	var rows []txRow

	baseWhere := "WHERE t.store_id = ? AND t.created_at >= ? AND t.created_at <= ?"
	args := []interface{}{storeID, startUTC, endUTC}

	// count
	var total int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM transactions t %s`, baseWhere)
	if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count transactions", err)
		return
	}

	dataSQL := fmt.Sprintf(`
		SELECT
			t.id,
			t.invoice,
			t.total_amount,
			t.total_item,
			t.total_quantity,
			t.payment_method,
			t.status,
			COALESCE(u.name, '') as user_name,
			t.created_at
		FROM transactions t
		LEFT JOIN users u ON u.id = t.user_id
		%s ORDER BY t.created_at DESC LIMIT ? OFFSET ?`, baseWhere)

	args = append(args, limit, offset)
	if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to fetch transactions", err)
		return
	}

	for i := range rows {
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, "Asia/Jakarta")
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

	c.JSON(http.StatusOK, response.Success("Transaction histories", gin.H{
		"data": rows,
		"pagination": pagination,
	}))
}
func StoreShiftsHistories(c *gin.Context) {
	idParam := c.Param("id")
	storeID, _ := strconv.Atoi(idParam)

	var store models.StoreProfile
	if err := config.DB.First(&store, storeID).Error; err != nil {
		helpers.ErrorResponse(c, 404, "store not found", err)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 { page = 1 }
	offset := (page - 1) * limit

	now, err := helpers.GetCurentTime("Asia/Jakarta")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
	// weekday := int(now.Weekday())
	// if weekday == 0 { weekday = 7 }
	start := time.Date(now.Year(), now.Month(), now.Day()-7, 0, 0, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), now.Location())
	startUTC := start.UTC()
	endUTC := end.UTC()

	type shiftRow struct {
		ID uint64 `json:"id"`
		CashierOpen string `json:"cashier_open"`
		CashierClosed string `json:"cashier_closed"`
		StartTime time.Time `json:"start_time"`
		EndTime *time.Time `json:"end_time"`
		Status string `json:"status"`
		InitialCash float64 `json:"initial_cash"`
		ExpectedCash float64 `json:"expected_cash"`
		ActualCash float64 `json:"actual_cash"`
		Difference float64 `json:"difference"`
		CreatedAt time.Time `json:"created_at"`
	}

	var rows []shiftRow

	baseWhere := "WHERE shifts.status = 'closed' AND shifts.store_id = ? AND shifts.created_at >= ? AND shifts.created_at <= ?"
	args := []interface{}{storeID, startUTC, endUTC}

	// count
	var total int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM shifts LEFT JOIN users u_open ON u_open.id = shifts.open_by LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by %s`, baseWhere)
	if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count shifts", err)
		return
	}

	dataSQL := fmt.Sprintf(`
		SELECT
			shifts.id,
			COALESCE(u_open.name, '-') as cashier_open,
			COALESCE(u_closed.name, '-') as cashier_closed,
			shifts.start_time,
			shifts.end_time,
			shifts.status,
			shifts.initial_cash,
			shifts.expected_cash,
			shifts.actual_cash,
			shifts.difference,
			shifts.created_at
		FROM shifts
		LEFT JOIN users u_open ON u_open.id = shifts.open_by
		LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by
		%s ORDER BY (shifts.status = 'open') DESC, shifts.created_at DESC LIMIT ? OFFSET ?`, baseWhere)

	args = append(args, limit, offset)
	if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to fetch shifts", err)
		return
	}

	for i := range rows {
		rows[i].StartTime = helpers.ToLocalTime(rows[i].StartTime, "Asia/Jakarta")
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, "Asia/Jakarta")
		if rows[i].EndTime != nil {
			end := helpers.ToLocalTime(*rows[i].EndTime, "Asia/Jakarta")
			rows[i].EndTime = &end
		}
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

	c.JSON(http.StatusOK, response.Success("Shifts histories", gin.H{
		"data": rows,
		"pagination": pagination,
	}))
}
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
