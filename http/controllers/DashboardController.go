package controllers

import (
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func GetDashboardData(c *gin.Context) {
	//total sale per hari
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
	// total stock (sum of quantity) across all products
	var totalAggregate struct {
		TotalStock int64   `json:"total_stock"`
		TotalPrice float64 `json:"total_price"`
	}
	if err := config.DB.Model(&models.Product{}).
		Where("status = ?", "display").
		Select(`
			COUNT(*) as total_stock,
			COALESCE(SUM(price),0) as total_price
		`).
		Scan(&totalAggregate).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to calculate", err)
		return
	}

	// stock per store
	type storeStockRow struct {
		StoreID   uint64 `json:"store_id"`
		StoreName string `json:"store_name"`
		Stock     int64  `json:"stock"`
	}

	var storeStocks []storeStockRow

	if err := config.DB.Raw(`
		SELECT 
			s.id as store_id,
			s.store_name as store_name,
			count(p.id) as stock
		FROM store_profiles s
		LEFT JOIN products p ON p.store_id = s.id
		WHERE p.status = 'display'
		GROUP BY s.id, s.store_name
	`).Scan(&storeStocks).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch stock per store", err)
		return
	}

	// recent transactions (limit 50)
	type txRow struct {
		ID              uint64    `json:"id"`
		Invoice         string    `json:"invoice"`
		StoreName       string    `json:"store_name"`
		TotalAmount     float64   `json:"total_amount"`
		Status          string    `json:"status"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var recentTx []txRow

	if err := config.DB.Raw(`
		SELECT 
			t.id,
			t.invoice,
			t.total_amount,
			t.status,
			t.created_at,
			s.store_name as store_name
		FROM transactions t
		JOIN store_profiles s ON s.id = t.store_id
		ORDER BY t.created_at DESC
		LIMIT 10
	`).Scan(&recentTx).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch recent transactions", err)
		return
	}

	for i := 0; i < len(recentTx); i++ {
		recentTx[i].CreatedAt = helpers.ToLocalTime(recentTx[i].CreatedAt, "Asia/Jakarta")
	}

	resp := gin.H{
		"total_sales": totalSales,
		"total_stock": totalAggregate.TotalStock,
		"total_price": totalAggregate.TotalPrice,
		"stock_per_store": storeStocks,
		"recent_transactions": recentTx,
	}

	c.JSON(http.StatusOK, response.Success("dashboard", resp))
}

func GetTotalSalesByFilter(c *gin.Context) {
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
				"label":		getDayIndo(d),
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

func getDayIndo(t time.Time) string {
	days := map[string]string{
		"Sunday":    "Minggu",
		"Monday":    "Senin",
		"Tuesday":   "Selasa",
		"Wednesday": "Rabu",
		"Thursday":  "Kamis",
		"Friday":    "Jumat",
		"Saturday":  "Sabtu",
	}
	return days[t.Format("Monday")]
}