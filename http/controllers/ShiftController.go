package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/http/response"
	"liquid8/pos/models"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func StartShift(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		InitialCash float64 `json:"initial_cash" binding:"required,gte=0"`
	}

	// Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
				"success": false, 
				"message": "Internal server error",
				"error": fmt.Sprintf("%v", r),
			})
        }
    }()

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}

		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "InitialCash":
				if e.Tag() == "required" {
					errorsMap["initial_cash"] = "Initial cash wajib diisi"
				} else if e.Tag() == "gte" {
					errorsMap["initial_cash"] = "Initial cash harus bernilai 0 atau lebih"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

	// determine store id
	var storeID uint64
	if user.StoreID != nil {
		storeID = *user.StoreID
	}

	// check session shift
	var existing models.Shift
	if err := config.DB.Where("status = ? AND store_id = ?", "open", storeID).First(&existing).Error; err == nil {
		helpers.ErrorResponse(c, http.StatusUnprocessableEntity, "Gagal: terdapat sesi shift masih aktif", nil)
		return
	}

	// prepare shift
	now, err := helpers.GetCurentTime(user.Store.Timezone)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
	shift := models.Shift{
		StoreID:     storeID,
		OpenBy:      uint64(user.ID),
		StartTime:   now.UTC(),
		Status:      "open",
		InitialCash: payload.InitialCash,
		ExpectedAmount: payload.InitialCash,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}

	if err := config.DB.Create(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal membuat shift", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("Shift created", shift))
}
func CurrentShift(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type resFormat struct {
		models.Shift
		ExpectedCash float64 `json:"expected_cash"`		
	}
	var shift models.Shift
	storeID := *user.StoreID

	if err := config.DB.Where("status = ? AND store_id = ?", "open", storeID).First(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Shift status open tidak ditemukan", err)
		return
	}

	result := resFormat{
		Shift: shift,
		ExpectedCash: shift.InitialCash + shift.TotalCash,
	}

	shift.ToLocal(user.Store.Timezone)
	c.JSON(http.StatusOK, response.Success("Current shift", result))
}
func EndShift(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payloadRequest struct {
		// ShiftID uint64 `json:"shift_id" binding:"required"`
		ActualCash float64 `json:"actual_cash" binding:"required,gte=0"`
		Note *string `json:"note" binding:"omitempty"`
	}

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}

		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "ActualCash":
				if e.Tag() == "required" {
					errorsMap["actual_cash"] = "actual cash wajib diisi"
				} else if e.Tag() == "gte" {
					errorsMap["actual_cash"] = "actual cash harus bernilai lebih dari 0"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}

	var shift models.Shift
	storeID := *user.StoreID

	if err := config.DB.
		Preload("Store").
		Preload("UserOpen").
		Where("status = ? AND store_id = ?", "open", storeID).First(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Shift status open tidak ditemukan", err)
		return
	}

	// if shift.Status == "closed" {
	// 	helpers.ErrorResponse(c, 422, "Shift ini sudah closed", nil)
	// 	return
	// }

	now, err := helpers.GetCurentTime(user.Store.Timezone)
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal mendapatkan waktu sekarang", err)
		return
	}
	
	summary, err := helpers.RecalculateTransactionShift(config.DB, storeID, shift.ID)
	if err != nil {
		helpers.ErrorResponse(c, 422, "Recalculate summary transaction shift failed", err)
		return
	}

	ExpectedAmount := shift.InitialCash + summary["total_amount"].(float64)
	ExpectedCash := summary["cash"].(float64) + shift.InitialCash
	diff := payload.ActualCash - (ExpectedCash)

	updates := map[string]interface{}{
		"end_time":   now.UTC(),
		"status":     "closed",

		"total_cash": summary["cash"].(float64),
		"total_transfer": summary["transfer"].(float64),
		"total_qris": summary["qris"].(float64),
		"total_tax": summary["tax_amount"].(float64),

		"subtotal": summary["subtotal"].(float64),
		"expected_amount": ExpectedAmount,
		"actual_cash": payload.ActualCash,
		"difference": diff,
		"closed_by":  uint64(user.ID),
		"note":  payload.Note,
	}

	if err := config.DB.Model(&shift).Updates(updates).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to close shift", err)
		return
	}
	shift.ToLocal(user.Store.Timezone)

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
    }

	result.Start = shift.StartTime
    result.End = shift.EndTime
    result.UserOpen = shift.UserOpen.Name
    result.UserClosed = user.Name

    result.InitialCash = shift.InitialCash
    result.TotalInvoice = summary["total_invoice"].(int64)
    
    result.TotalCash = shift.TotalCash
    result.TotalTransfer = shift.TotalTransfer
    result.TotalQris = shift.TotalQris
    result.TotalCashCancel = summary["total_cash_cancel"].(float64)
    result.TotalTransferCancel = summary["total_transfer_cancel"].(float64)
    result.TotalQrisCancel = summary["total_qris_cancel"].(float64)
    
    result.TotalTax = shift.TotalTax
    result.TotalSubtotal = shift.Subtotal
    result.TotalAmount = summary["total_amount"].(float64)
    result.TotalRounded = summary["total_rounded"].(float64)
    result.ExpectedCash = shift.TotalCash + shift.InitialCash
    result.ExpectedAmount = shift.ExpectedAmount
    result.ActualCash = shift.ActualCash
    result.ActualAmount = shift.ExpectedAmount + shift.Difference
    result.Difference = shift.Difference
    result.Note = shift.Note
    
    result.Store.Name = shift.Store.StoreName
    result.Store.Phone = shift.Store.Phone
    result.Store.Address = shift.Store.Address

	c.JSON(http.StatusOK, response.Success("Shift closed", result))
}
func GetShiftsByCashier(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	q := c.DefaultQuery("q", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	startDateQuery := c.Query("start_date")
	endDateQuery := c.Query("end_date")

	getStartOfDay := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}

	getEndOfDay := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
	}

	// response row structure
	type shiftRow struct {
		ID           uint64    `json:"id"`
		CashierOpen  string    `json:"cashier_open"`
		CashierClosed string   `json:"cashier_closed"`
		StartTime    time.Time `json:"start_time"`
		EndTime      *time.Time `json:"end_time"`
		Status       string    `json:"status"`
		InitialCash  float64   `json:"initial_cash"`
		TotalCash	float64		`json:"total_cash"`
		TotalTransfer	float64		`json:"total_transfer"`
		TotalQris	float64		`json:"total_qris"`
		ExpectedCash	float64 	`json:"expected_cash"`
		ExpectedAmount float64   `json:"expected_amount"`
		ActualCash   float64   `json:"actual_cash"`
		Difference   float64   `json:"difference"`
		StoreName    string    `json:"store_name"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var rows []shiftRow

	// build base where
	whereClauses := "(shifts.open_by = ? OR shifts.status = 'open')"
	args := []interface{}{user.ID}
	if user.StoreID != nil {
		whereClauses += " AND shifts.store_id = ?"
		args = append(args, *user.StoreID)
	}

	var startUTC, endUTC *time.Time
	if startDateQuery != "" {
		start, err := helpers.ParseFlexibleDate(startDateQuery, user.Store.Timezone)
		if err != nil {
			helpers.ErrorResponse(c, 400, "Invalid start_date", err)
			return
		}
		s := getStartOfDay(start).UTC()
		startUTC = &s
	}
	if endDateQuery != "" {
		end, err := helpers.ParseFlexibleDate(endDateQuery, user.Store.Timezone)
		if err != nil {
			helpers.ErrorResponse(c, 400, "Invalid end_date", err)
			return
		}
		e := getEndOfDay(end).UTC()
		endUTC = &e
	}
	// default: hari ini
	// if startUTC == nil && endUTC == nil {
	// 	now := helpers.GetCurentTime(user.Store.Timezone)
	// 	s := getStartOfDay(now).UTC()
	// 	e := getEndOfDay(now).UTC()

	// 	startUTC = &s
	// 	endUTC = &e
	// }
	// ==================
	// APPLY TO QUERY
	// ==================
	if startUTC != nil {
		whereClauses += " AND shifts.created_at >= ?"
		args = append(args, *startUTC)
	}
	if endUTC != nil {
		whereClauses += " AND shifts.created_at <= ?"
		args = append(args, *endUTC)
	}


	if q != "" {
		like := "%" + q + "%"
		whereClauses += " AND (u_open.name LIKE ? OR u_closed.name LIKE ?)"
		args = append(args, like, like)
	}

	// count
	countQuery := fmt.Sprintf(`SELECT count(*) FROM shifts
		LEFT JOIN users u_open ON u_open.id = shifts.open_by
		LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by
		LEFT JOIN store_profiles s ON s.id = shifts.store_id
		WHERE %s`, whereClauses)

	var totalData int64
	if err := config.DB.Raw(countQuery, args...).Scan(&totalData).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to count shifts", err)
		return
	}

	// data query
	dataQuery := fmt.Sprintf(`SELECT
		shifts.id,
		COALESCE(u_open.name, '-') as cashier_open,
		COALESCE(u_closed.name, '-') as cashier_closed,
		shifts.start_time,
		shifts.end_time,
		shifts.status,
		shifts.initial_cash,
		shifts.total_cash,
		shifts.total_transfer,
		shifts.total_qris,
		COALESCE(shifts.total_cash + shifts.initial_cash, 0) as expected_cash,
		shifts.expected_amount,
		shifts.actual_cash,
		shifts.difference,
		COALESCE(s.store_name, '') as store_name,
		shifts.created_at
		FROM shifts
		LEFT JOIN users u_open ON u_open.id = shifts.open_by
		LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by
		LEFT JOIN store_profiles s ON s.id = shifts.store_id
		WHERE %s
		ORDER BY (shifts.status = 'open') DESC, shifts.created_at DESC
		LIMIT ? OFFSET ?`, whereClauses)

	args = append(args, limit, offset)
	if err := config.DB.Raw(dataQuery, args...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to get shifts", err)
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

	for i := range rows {
		rows[i].StartTime = helpers.ToLocalTime(rows[i].StartTime, user.Store.Timezone)
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, user.Store.Timezone)
		if rows[i].EndTime != nil {
			endTime := helpers.ToLocalTime(*rows[i].EndTime, user.Store.Timezone)
			rows[i].EndTime = &endTime
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"message": "List data shifts",
		"resource": gin.H{
			"data": rows,
			"pagination": pagination,
		},
	})
}
func GetAllShifts(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)
	q := c.DefaultQuery("q", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	// response row structure
	type shiftRow struct {
		ID           uint64    `json:"id"`
		CashierOpen  string    `json:"cashier_open"`
		CashierClosed string   `json:"cashier_closed"`
		StartTime    time.Time `json:"start_time"`
		EndTime      *time.Time `json:"end_time"`
		Status       string    `json:"status"`
		InitialCash  float64   `json:"initial_cash"`
		ExpectedCash float64   `json:"expected_cash"`
		ActualCash   float64   `json:"actual_cash"`
		Difference   float64   `json:"difference"`
		StoreName    string    `json:"store_name"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var rows []shiftRow

	// build base where
	whereClauses := ""
	args := []interface{}{}

	// count
	countQuery := `
		SELECT count(*) FROM shifts
		LEFT JOIN users u_open ON u_open.id = shifts.open_by
		LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by
		LEFT JOIN store_profiles s ON s.id = shifts.store_id
	`

	// data query
	dataQuery := `
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
			COALESCE(s.store_name, '') as store_name,
			shifts.created_at
		FROM shifts
		LEFT JOIN users u_open ON u_open.id = shifts.open_by
		LEFT JOIN users u_closed ON u_closed.id = shifts.closed_by
		LEFT JOIN store_profiles s ON s.id = shifts.store_id
	`

	if q != "" {
		like := "%" + q + "%"
		whereClauses += "(u_open.name LIKE ? OR u_closed.name LIKE ? OR s.store_name LIKE ?)"
		countQuery += fmt.Sprintf("WHERE %s", whereClauses)
		dataQuery += fmt.Sprintf("WHERE %s", whereClauses)

		args = append(args, like, like, like)
	}

	var totalData int64
	if err := config.DB.Raw(countQuery, args...).Scan(&totalData).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to count shifts", err)
		return
	}

	dataQuery += `
		ORDER BY (shifts.status = 'open') DESC, shifts.created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)
	if err := config.DB.Raw(dataQuery, args...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to get shifts", err)
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(totalData))

	timezone := "Asia/Jakarta"
	if user.StoreID != nil {
		timezone = user.Store.Timezone
	}
	for i := range rows {
		rows[i].StartTime = helpers.ToLocalTime(rows[i].StartTime, timezone)
		rows[i].CreatedAt = helpers.ToLocalTime(rows[i].CreatedAt, timezone)
		if rows[i].EndTime != nil {
			endTime := helpers.ToLocalTime(*rows[i].EndTime, timezone)
			rows[i].EndTime = &endTime
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"message": "List data shifts",
		"resource": gin.H{
			"data": rows,
			"pagination": pagination,
		},
	})
}
