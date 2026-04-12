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
	now := helpers.GetCurentTime()
	shift := models.Shift{
		StoreID:     storeID,
		OpenBy:      uint64(user.ID),
		StartTime:   now,
		Status:      "open",
		InitialCash: payload.InitialCash,
	}

	if err := config.DB.Create(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal membuat shift", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("Shift created", shift))
}
func CurrentShift(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	var shift models.Shift
	storeID := *user.StoreID

	if err := config.DB.Where("status = ? AND store_id = ?", "open", storeID).First(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Shift status open tidak ditemukan", err)
		return
	}

	c.JSON(http.StatusOK, response.Success("Current shift", shift))
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
					errorsMap["actual_cash"] = "actual cash harus bernilai 0 atau lebih"
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

	if err := config.DB.Where("status = ? AND store_id = ?", "open", storeID).First(&shift).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Shift status open tidak ditemukan", err)
		return
	}

	// if shift.Status == "closed" {
	// 	helpers.ErrorResponse(c, 422, "Shift ini sudah closed", nil)
	// 	return
	// }

	now := helpers.GetCurentTime()
	expectedCash, err := helpers.RecalculateShiftExpectedCash(config.DB, storeID, shift.ID)
	if err != nil {
		helpers.ErrorResponse(c, 422, "Recalculate expected cash gagal", err)
		return
	}

	expectedCash = shift.InitialCash + expectedCash
	diff := payload.ActualCash - expectedCash

	updates := map[string]interface{}{
		"end_time":   now,
		"status":     "closed",
		"actual_cash": payload.ActualCash,
		"expected_cash": expectedCash,
		"difference": diff,
		"closed_by":  uint64(user.ID),
		"note":  payload.Note,
	}

	if err := config.DB.Model(&shift).Updates(updates).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to close shift", err)
		return
	}

	// reload
	config.DB.First(&shift, shift.ID)

	c.JSON(http.StatusOK, response.Success("Shift closed", shift))
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
	whereClauses := "(shifts.open_by = ? OR shifts.status = 'open')"
	args := []interface{}{user.ID}
	if user.StoreID != nil {
		whereClauses += " AND shifts.store_id = ?"
		args = append(args, *user.StoreID)
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
		shifts.expected_cash,
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

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"message": "List data shifts",
		"resource": gin.H{
			"data": rows,
			"pagination": pagination,
		},
	})
}
