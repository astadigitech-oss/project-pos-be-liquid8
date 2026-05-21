package controllers

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
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
			COALESCE(p.total_product, 0) as total_product,
			COALESCE(t.total_sales, 0) as total_sales
		`).
		Joins(`
			LEFT JOIN (
				SELECT store_id, COUNT(*) as total_product
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
	var total int64
	if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count stores", err)
		return
	}

	// fetch data
	if err := db.Limit(limit).Offset(offset).Scan(&results).Error; err != nil {
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
func ListStoresDropdown(c *gin.Context) {
	type formatRes struct {
		ID              uint       `json:"id"`
		StoreName		string	   `json:"store_name"`
	}

	var results []formatRes

	if err := config.DB.Model(&models.StoreProfile{}).Select(`
			id,
			store_name
		`).Scan(&results).Error; err != nil {

		helpers.ErrorResponse(c, 500, "Gagal mengambil store list", err);
		return
	}

	c.JSON(http.StatusOK, results)
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

	type Row struct {
		Date  time.Time
		Total float64
	}

	var rows []Row
	baseQuery := config.DB.Model(&models.Transaction{}).
		Select("DATE(created_at + INTERVAL 7 HOUR) as date, COALESCE(SUM(total_amount),0) as total").
		Where("store_id = ?", storeID).
		Where("status = ?", "done")

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
			end = monday.AddDate(0, 0, 7).Add(-time.Second)

			baseQuery.Where("created_at >= ? AND created_at <= ?", start.UTC(), end.UTC())

		case "month":
			start = time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
			end = time.Date(now.Year(), time.December, 31, 23, 59, 59, 0, now.Location())

			baseQuery.Where("created_at >= ? AND created_at <= ?", start.UTC(), end.UTC())
		
		case "custom":
			startDateQuery := c.Query("start_date")
			endDateQuery := c.Query("end_date")

			if startDateQuery == "" || endDateQuery == "" {
				helpers.ErrorResponse(c, 400, "Date range harus valid untuk period custom", nil)
				return
			}

			startDate, err := helpers.ParseFlexibleDate(startDateQuery, "Asia/Jakarta")
			if err != nil {
				helpers.ErrorResponse(c, 400, "Invalid start_date", err)
				return
			}
			endDate, err := helpers.ParseFlexibleDate(endDateQuery, "Asia/Jakarta")
			if err != nil {
				helpers.ErrorResponse(c, 400, "Invalid end_date", err)
				return
			}

			start = helpers.GetStartOfDay(startDate)
			end = helpers.GetEndOfDay(endDate)

			baseQuery.Where("created_at >= ? AND created_at <= ?", start.UTC(), end.UTC())

		default:
			helpers.ErrorResponse(c, 400, "Invalid period", nil)
			return
	}

	if err := baseQuery.Group("date").Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch sales data", err)
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
	switch period {
		case "week":
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
		case"custom":
			payload.Start = start.Format("02 January 2006")
			payload.End = end.Format("02 January 2006")

			for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {

				key := d.Format("2006-01-02")

				results = append(results, gin.H{
					"label":	d.Format("02 January 2006"),
					"date":     d.Format("02 January 2006"),
					"total_sales": resultMap[key], // default 0 kalau tidak ada
				})
			}
		case "month":
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
func ExportDetailStore(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var store models.StoreProfile
	if err := config.DB.First(&store, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "store not found", err)
		return
	}

	// total sales (all-time, done)
	var totalSales float64
	if err := config.DB.Raw(`SELECT COALESCE(SUM(total_amount),0) as total_sales FROM transactions WHERE status = 'done' AND store_id = ?`, id).Scan(&totalSales).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to calculate total sales", err)
		return
	}

	// aggregate products by tag_color
	type colorAgg struct {
		TagColor   string  `json:"tag_color"`
		TotalStock int64   `json:"total_stock"`
		TotalPrice float64 `json:"total_price"`
	}
	var colorAggs []colorAgg
	if err := config.DB.Raw(`
		SELECT COALESCE(tag_color, '') AS tag_color, COUNT(*) AS total_stock, COALESCE(SUM(price),0) AS total_price
		FROM products
		WHERE store_id = ? AND status = 'display'
		GROUP BY tag_color
	`, id).Scan(&colorAggs).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to aggregate products by color", err)
		return
	}

	// fetch products
	type prodRow struct {
		ID        uint64    `json:"id"`
		OldBarcode   string    `json:"old_barcode"`
		Barcode   string    `json:"barcode"`
		Name      string    `json:"name"`
		Price     float64   `json:"price"`
		TagColor  string    `json:"tag_color"`
		Quantity  int64     `json:"quantity"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}
	var products []prodRow
	prodSQL := `
		SELECT id, COALESCE(old_barcode, "") as old_barcode, barcode, name, price, COALESCE(tag_color,'') AS tag_color, quantity, status, created_at
		FROM products
		WHERE store_id = ? AND status = 'display'
		ORDER BY created_at DESC
	`
	if err := config.DB.Raw(prodSQL, id).Scan(&products).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch products", err)
		return
	}

	// fetch transactions
	type txItemRow struct {
        ID uint64 
        Invoice string
        Status string 
        Kasir string 
        OldBarcode string
        Barcode string
        ProductName string
        TagColor string
        Quantity uint64
        Price float64
        PaymentMethod string
        Type string
        CreatedAt time.Time
    }
	var txs []txItemRow
	txSQL := `
        SELECT 
            ti.id,
            t.invoice,
            t.status,
            COALESCE(u.name, "-") as kasir,
            COALESCE(p.old_barcode, "") as old_barcode,
            COALESCE(p.barcode, "") as barcode,
            ti.product_name,
			p.tag_color,
            ti.quantity,
            ti.price,
            ti.subtotal,
			t.payment_method,
            ti.type,
            t.created_at
        FROM transaction_items ti
        JOIN transactions t ON t.id = ti.transaction_id
        LEFT JOIN users u ON u.id = t.user_id
        LEFT JOIN products p ON p.id = ti.product_id
        WHERE t.store_id = ?
        ORDER BY ti.created_at DESC
    `
	if err := config.DB.Raw(txSQL, id).Scan(&txs).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch transactions", err)
		return
	}
	//ubah ke local time
    for i := range txs {
        txs[i].CreatedAt = helpers.ToLocalTime(txs[i].CreatedAt, "Asia/Jakarta")
    }

	//grouping per kategori
	type groupingItemPrice struct {
        Name string `json:"name"`
        Price    float64 `json:"price"`
        Quantity uint64   `json:"quantity"`
        Total float64   `json:"total"`
    }
    priceMap := make(map[float64]uint64)
	var totalProductSale uint64
    var productSale []groupingItemPrice
    for _, item := range txs {
        if item.Status == "done" {
            switch item.Type {
            case "product":
                totalProductSale += 1
                priceMap[item.Price] += item.Quantity
            }
        }
    }
    // ambil item packaging
    for price, qty := range priceMap {
        productSale = append(productSale, groupingItemPrice{
            Name: formatPriceToProductName(price),
            Price:    price,
            Quantity: qty,
            Total: price * float64(qty),
        })
    }
    // sorting dari harga terendah
    sort.Slice(productSale, func(i, j int) bool {
        return productSale[i].Price < productSale[j].Price
    })


	// build excel
	f := excelize.NewFile()
	defer f.Close()
	sheet1 := "Summary"
	sheet2 := "Produk"
	sheet3 := "Penjualan"

	// rename default sheet to Detail
	f.SetSheetName("Sheet1", sheet1)

	fillGrayStyle, _ := helpers.BuildStyle(f, config.ExcelStyles["fill_gray"], config.ExcelStyles["font_bold"])
	fontHeaderStyle,_ := helpers.BuildStyle(f, config.ExcelStyles["font_bold_size14"])
	fontBold,_ := helpers.BuildStyle(f, config.ExcelStyles["font_bold"])

	// Sheet1: store info
	f.MergeCell(sheet1, "A1", "D1")
	f.MergeCell(sheet1, "A2", "D2")
	f.MergeCell(sheet1, "A3", "D3")
	f.MergeCell(sheet1, "A5", "C5")
	f.MergeCell(sheet1, "A6", "B6")
	f.SetColWidth(sheet1, "A", "D", 16)

	// f.SetColWidth(sheet1, "A", "C", 20)

	f.SetCellValue(sheet1, "A1", store.StoreName)
	f.SetCellStyle(sheet1, "A1", "B1", fontHeaderStyle)
	f.SetCellValue(sheet1, "A2", store.Address)
	f.SetCellValue(sheet1, "A3", store.Phone)

	f.SetCellValue(sheet1, "A5", "Summary Produk")
	f.SetCellStyle(sheet1, "A5", "A5", fontBold)
	f.SetCellValue(sheet1, "A6", "Total Produk")
	f.SetCellValue(sheet1, "C6", len(products))
	// f.SetCellValue(sheet1, "B5", totalSales)

	// color aggregates header starting row 7
	f.SetCellValue(sheet1, "A8", "Kategori")
	f.SetCellValue(sheet1, "B8", "Total Stock")
	f.SetCellValue(sheet1, "C8", "Total Price")
	f.SetCellStyle(sheet1, "A8", "C8", fillGrayStyle)
	r := 9
	for _, ca := range colorAggs {
		cellA, _ := excelize.CoordinatesToCellName(1, r)
		cellB, _ := excelize.CoordinatesToCellName(2, r)
		cellC, _ := excelize.CoordinatesToCellName(3, r)
		f.SetCellValue(sheet1, cellA, ca.TagColor)
		f.SetCellValue(sheet1, cellB, ca.TotalStock)
		f.SetCellValue(sheet1, cellC, ca.TotalPrice)
		r++
	}
	r++
	f.MergeCell(sheet1, fmt.Sprintf("A%d", r), fmt.Sprintf("C%d", r))
	f.SetCellStyle(sheet1, fmt.Sprintf("A%d", r), fmt.Sprintf("C%d", r), fontBold)
	f.SetCellValue(sheet1, fmt.Sprintf("A%d", r), "Summary Transaction")
	r++
	f.MergeCell(sheet1, fmt.Sprintf("A%d", r), fmt.Sprintf("B%d", r))
	f.SetCellValue(sheet1, fmt.Sprintf("A%d", r), "Total Produk Terjual")
	f.SetCellValue(sheet1, fmt.Sprintf("C%d", r), totalProductSale)
	r += 2
	// summary produk terjual per kategori
	f.SetCellValue(sheet1, fmt.Sprintf("A%d", r), "Kategori")
	f.SetCellValue(sheet1, fmt.Sprintf("B%d", r), "Harga")
	f.SetCellValue(sheet1, fmt.Sprintf("C%d", r), "Quantity")
	f.SetCellValue(sheet1, fmt.Sprintf("D%d", r), "Total Price")
	f.SetCellStyle(sheet1, fmt.Sprintf("A%d", r), fmt.Sprintf("D%d", r), fillGrayStyle)
	r++
	for _, ps := range productSale {
		cellA, _ := excelize.CoordinatesToCellName(1, r)
		cellB, _ := excelize.CoordinatesToCellName(2, r)
		cellC, _ := excelize.CoordinatesToCellName(3, r)
		cellD, _ := excelize.CoordinatesToCellName(4, r)
		f.SetCellValue(sheet1, cellA, ps.Name)
		f.SetCellValue(sheet1, cellB, ps.Price)
		f.SetCellValue(sheet1, cellC, ps.Quantity)
		f.SetCellValue(sheet1, cellD, ps.Total)
		r++
	}

	//sheet2: Produk
	f.NewSheet(sheet2)
	f.SetCellStyle(sheet2, fmt.Sprintf("A%d", r), fmt.Sprintf("A%d", r), fontBold)
	f.SetColWidth(sheet2, "A", "I", 20)
	f.SetColWidth(sheet2, "A", "A", 5)
	f.SetColWidth(sheet2, "D", "D", 30)
	headers := []string{"ID", "Old Barcode", "Barcode", "Name", "Price", "TagColor", "Quantity", "Status", "CreatedAt"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet2, cell, h)
		f.SetCellStyle(sheet2, cell, cell, fillGrayStyle)
	}
	for i, p := range products {
		row := i + 2
		f.SetCellValue(sheet2, fmt.Sprintf("A%d", row), p.ID)
		f.SetCellValue(sheet2, fmt.Sprintf("B%d", row), p.OldBarcode)
		f.SetCellValue(sheet2, fmt.Sprintf("C%d", row), p.Barcode)
		f.SetCellValue(sheet2, fmt.Sprintf("D%d", row), p.Name)
		f.SetCellValue(sheet2, fmt.Sprintf("E%d", row), p.Price)
		f.SetCellValue(sheet2, fmt.Sprintf("F%d", row), p.TagColor)
		f.SetCellValue(sheet2, fmt.Sprintf("G%d", row), p.Quantity)
		f.SetCellValue(sheet2, fmt.Sprintf("H%d", row), p.Status)
		f.SetCellValue(sheet2, fmt.Sprintf("I%d", row), helpers.ToLocalTime(p.CreatedAt, store.Timezone).Format("2006-01-02 15:04:05"))
	}

	// Sheet3: transactions
	f.NewSheet(sheet3)
	f.SetColWidth(sheet3, "A", "L", 20)
	f.SetColWidth(sheet3, "A", "A", 5)
	th := []string{"No", "Invoice", "Kasir", "Old Barcode", "Barcode", "Product Name", "Category", "Price", "Type", "Payment Method", "Status", "Transaction Date"}
	for i, h := range th {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet3, cell, h)
		f.SetCellStyle(sheet3, cell, cell, fillGrayStyle)
	}
	for i, t := range txs {
		row := i + 2
		f.SetCellValue(sheet3, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet3, fmt.Sprintf("B%d", row), t.Invoice)
		f.SetCellValue(sheet3, fmt.Sprintf("C%d", row), t.Kasir)
		f.SetCellValue(sheet3, fmt.Sprintf("D%d", row), t.OldBarcode)
		f.SetCellValue(sheet3, fmt.Sprintf("E%d", row), t.Barcode)
		f.SetCellValue(sheet3, fmt.Sprintf("F%d", row), t.ProductName)
		f.SetCellValue(sheet3, fmt.Sprintf("G%d", row), t.TagColor)
		f.SetCellValue(sheet3, fmt.Sprintf("H%d", row), t.Price)
		f.SetCellValue(sheet3, fmt.Sprintf("I%d", row), t.Type)
		f.SetCellValue(sheet3, fmt.Sprintf("J%d", row), t.PaymentMethod)
		f.SetCellValue(sheet3, fmt.Sprintf("K%d", row), t.Status)
		f.SetCellValue(sheet3, fmt.Sprintf("L%d", row), helpers.ToLocalTime(t.CreatedAt, "Asia/Jakarta").Format("2006-01-02 15:04:05"))
	}

	// set active sheet to Detail
	idx, _ := f.GetSheetIndex(sheet1)
	f.SetActiveSheet(idx)

	filename := fmt.Sprintf("%s_%d.xlsx", strings.ToLower(store.StoreName), time.Now().Unix())
	filename = strings.ReplaceAll(filename, " ", "_")
	// Save file
	dir := "./public/exports"
	if err := os.MkdirAll(dir, 0755); err != nil {
		helpers.ErrorResponse(c, 500, "Failed create directory", err)
		return
	}

	prefix := fmt.Sprintf("%s_", strings.ToLower(store.StoreName))
	prefix = strings.ReplaceAll(prefix, " ", "_")
	files, err := os.ReadDir(dir)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Failed read directory", err)
		return
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filename := file.Name()
		// cek awalan nama file
		if strings.HasPrefix(filename, prefix) {
			fullPath := filepath.Join(dir, filename)

			if err := os.Remove(fullPath); err != nil {
				fmt.Println("failed remove file:", fullPath, err)
				continue
			}

			fmt.Println("deleted:", fullPath)
		}
	}

	fullPath := filepath.Join(dir, filename)

	if err := f.SaveAs(fullPath); err != nil {
		helpers.ErrorResponse(c, 500, "Gagal menyimpan file", err)
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), filename)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunduh",
		"url":     downloadURL,
	})
}