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
	var totalSales float64
	if err := config.DB.Raw(`
		SELECT 
			COALESCE(SUM(total_amount), 0) as total_sales
		FROM transactions
		WHERE status = 'done'
		AND transaction_date = CURDATE()
	`).Scan(&totalSales).Error; err != nil {
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
		TransactionDate string    `json:"transaction_date"`
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
			t.transaction_date,
			t.created_at,
			s.store_name as store_name
		FROM transactions t
		JOIN store_profiles s ON s.id = t.store_id
		ORDER BY t.created_at DESC
		LIMIT 30
	`).Scan(&recentTx).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch recent transactions", err)
		return
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

	now := helpers.GetCurentTime()

	var start time.Time
	var end time.Time

	switch period {
	case "week":
		// Week: Monday..Sunday of current week
		// time.Sunday    = 0
		// time.Monday    = 1
		// time.Tuesday   = 2
		// time.Wednesday = 3
		// time.Thursday  = 4
		// time.Friday    = 5
		// time.Saturday  = 6
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday -> make it 7
			weekday = 7
		}
		// compute monday
		//now.Day() tgl 1-31
		monday := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
		start = monday
		end = monday.AddDate(0, 0, 6).Add(-time.Nanosecond)
	case "month":
		// month means full year range: Jan 1 .. Dec 31 of current year
		start = time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
		end = time.Date(now.Year()+1, time.January, 1, 0, 0, 0, 0, now.Location()).Add(-time.Nanosecond)
	default:
		// day (today)
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}

	type DailySales struct {
		Date  string  `json:"date"`
		Total float64 `json:"total"`
	}

	var results []DailySales

	if err := config.DB.Model(&models.Transaction{}).
		Select("DATE(transaction_date) as date, COALESCE(SUM(total_amount),0) as total").
		Where("status = ? AND transaction_date >= ? AND transaction_date <= ?", "done", start.Format("2006-01-02"), end.Format("2006-01-02")).
		Group("DATE(transaction_date)").
		Order("date ASC").
		Scan(&results).Error; err != nil {

		helpers.ErrorResponse(c, 500, "Failed to calculate daily sales", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("total sales", gin.H{
		"period": period,
		"start": start.Format("2006-01-02"),
		"end": end.Format("2006-01-02"),
		"sales": results,
	}))
}