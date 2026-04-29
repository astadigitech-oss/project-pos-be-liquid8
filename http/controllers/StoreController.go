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
	// =======================================
	// SUMMARY STORE
	// =======================================
	id, _ := strconv.Atoi(c.Param("id"))
	var store models.StoreProfile
	if err := config.DB.First(&store, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "store not found", err)
		return
	}

	// TOTAL SALES TODAY
	now, err := helpers.GetCurentTime("Asia/Jakarta")
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24*time.Hour - time.Nanosecond)
	// convert ke UTC
	startUTC := start.UTC()
	endUTC := end.UTC()

	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mengambil current time", err)
		return
	}
	var totalSales float64
	if err := config.DB.Raw(`
		SELECT 
			COALESCE(SUM(total_amount), 0) as total_sales
		FROM transactions
		WHERE status = 'done'
		AND created_at BETWEEN ? AND ?
	`, startUTC, endUTC).Scan(&totalSales).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch today sales", err)
		return
	}

	// GET TOTAL STOCK AND PRICE PRODUK
	var totalAggregate struct {
		TotalStock int64   `json:"total_stock"`
		TotalPrice float64 `json:"total_price"`
	}
	if err := config.DB.Model(&models.Product{}).
		Where("store_id = ?", store.ID).
		Where("status = ?", "display").
		Select(`
			COUNT(*) as total_stock,
			COALESCE(SUM(price),0) as total_price
		`).
		Scan(&totalAggregate).Error; err != nil {

		helpers.ErrorResponse(c, 500, "Failed to calculate total stock and price produk in store", err)
		return
	}

	// ==============================
	// GET PRODUK STORE WITH PAGINATION
	// ==============================
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
	// select data produk
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
	//convert to timezone store
	for i := range rows {
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, store.Timezone)
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

	c.JSON(http.StatusOK, response.Success("Detail store", gin.H{
		"store": gin.H{
			"id": store.ID,
			"store_name": store.StoreName,
			"phone": store.Phone,
			"address": store.Address,
			"total_sales_today": totalSales,
			"total_stock": totalAggregate.TotalStock,
			"total_price_product": totalAggregate.TotalPrice,
		},
		"products": gin.H{
			"data": rows, 
			"pagination": pagination,
		},
	}))
}
func GetSalePeriodStore(c *gin.Context) {
	idParam := c.Param("id")
	storeID, _ := strconv.Atoi(idParam)

	period := c.DefaultQuery("period", "week")

	now, err := helpers.GetCurentTime("Asia/Jakarta")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}

	var start time.Time
	var end time.Time

	switch period {
	case "week":
		// Week: Monday..Sunday of current week
		// time.Sunday = 0
		// time.Monday = 1
		// time.Tuesday = 2
		// time.Wednesday = 3
		// time.Thursday = 4
		// time.Friday = 5
		// time.Saturday = 6
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
		start = monday
		end = monday.AddDate(0, 0, 5)

	case "month":
		start = time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
		end = time.Date(now.Year(), time.December, 31, 23, 59, 59, 0, now.Location())

	default:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1).Add(-time.Second)
	}

	type Row struct {
		Date  time.Time
		Total float64
	}

	var rows []Row
	if err := config.DB.Model(&models.Transaction{}).
		Select("DATE(created_at + INTERVAL 7 HOUR) as date, COALESCE(SUM(total_amount),0) as total").
		Where("store_id = ?", storeID).
		Where("status = ? AND created_at >= ? AND created_at <= ?", "done", start.UTC(), end.UTC()).
		Group("date").
		Scan(&rows).Error; err != nil {

		helpers.ErrorResponse(c, 500, "Failed to calculate sales", err)
		return
	}

	// Mapping hasil query
	resultMap := make(map[string]float64)
	for _, r := range rows {
		key := r.Date.Format("2006-01-02")
		resultMap[key] = r.Total
	}

	// Final result
	var results []gin.H
	type resFormat struct {
		Period    string  `json:"period"`
		Start     string  `json:"start"`
		End       string  `json:"end"`
		Sales     []gin.H `json:"sales"`
	}
	payload := resFormat{
		Period: period,
	}
	// WEEK / DAY (per hari)
	if period == "week" || period == "day" {
		payload.Start = start.Format("02 January 2006")
		payload.End = end.Format("02 January 2006")

		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {

			key := d.Format("2006-01-02")

			results = append(results, gin.H{
				"label":		helpers.GetDayIndo(d),
				"date":        d.Format("02 January 2006"),
				"total_sales": resultMap[key], // default 0 kalau tidak ada
			})
		}
	}

	// MONTH (per bulan)
	if period == "month" {
		payload.Start = start.Format("January 2006")
		payload.End = end.Format("January 2006")

		monthMap := make(map[int]float64)

		// grouping ulang per bulan
		for _, r := range rows {
			month := int(r.Date.Month())
			monthMap[month] += r.Total
		}

		for m := 1; m <= 12; m++ {

			d := time.Date(now.Year(), time.Month(m), 1, 0, 0, 0, 0, now.Location())

			results = append(results, gin.H{
				"date":        d.Format("January 2006"),
				"label":		d.Format("January"),
				"total_sales": monthMap[m],
			})
		}
	}
	payload.Sales = results

	c.JSON(http.StatusOK, response.Success("total sales", payload))
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
