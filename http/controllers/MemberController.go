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

func ListMembers(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

    q := strings.TrimSpace(c.DefaultQuery("q", ""))
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
    if page < 1 { page = 1 }
    offset := (page-1)*limit

    baseWhere := "WHERE deleted_at IS NULL"
    args := []interface{}{}
	if user.StoreID != nil {
		baseWhere += " AND store_id = ?"
		args = append(args, *user.StoreID)
	}

    if q != "" { 
		baseWhere += " AND (name LIKE ? OR phone LIKE ?)"; 
		like := "%"+q+"%"; 
		args = append(args, like, like) 
	}

    var total int64
    countSQL := fmt.Sprintf("SELECT COUNT(*) FROM members %s", baseWhere)
    if len(args) > 0 { 
		if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	} else {
		if err := config.DB.Raw(countSQL).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	}

    type Row struct {
        ID uint `json:"id"`
        Code string `json:"code"`
        Name string `json:"name"`
        Phone string `json:"phone"`
    }
    var rows []Row

    dataSQL := fmt.Sprintf(`
		SELECT 
			id, 
			code, 
			name,  
			phone 
		FROM members %s 
		ORDER BY created_at DESC LIMIT ? OFFSET ?`, baseWhere)
    args = append(args, limit, offset)
    if err := config.DB.Raw(dataSQL, args...).Scan(&rows).Error; err != nil { helpers.ErrorResponse(c, 500, "Failed to fetch members", err); return }

    lastPage := int(math.Ceil(float64(total)/float64(limit)))
    pagination := helpers.BuildPaginationLinks(c, page, limit, lastPage, len(rows), int(total))

    c.JSON(http.StatusOK, response.Success("Members", gin.H{"data": rows, "pagination": pagination}))
}
func ListAllMembers(c *gin.Context) {
	q := strings.TrimSpace(c.DefaultQuery("q", ""))
	storeIDParam := strings.TrimSpace(c.DefaultQuery("store_id", ""))

	// filter periode
	monthParam := strings.TrimSpace(c.DefaultQuery("month", ""))
	yearParam := strings.TrimSpace(c.DefaultQuery("year", ""))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))

	// sort_by: name | store | point
	sortBy := strings.TrimSpace(c.DefaultQuery("sort_by", "created_at"))

	// sort_type: asc | desc
	sortType := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("sort_type", "desc")))
	if strings.ToLower(sortType) != "asc" && strings.ToLower(sortType) != "desc" {
		sortType = "desc"
	}

	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 30
	}

	offset := (page - 1) * limit

	var store models.StoreProfile

	// =========================
	// BASE QUERY
	// =========================
	baseWhere := `WHERE m.deleted_at IS NULL`
	args := []interface{}{}

	// =========================
	// FILTER STORE ID
	// =========================
	if storeIDParam != "" {
		storeID, err := strconv.Atoi(storeIDParam)
		if err != nil {
			helpers.ErrorResponse(c, 400, "invalid store_id", err)
			return
		}

		if err := config.DB.First(&store, storeID).Error; err != nil {
			helpers.ErrorResponse(c, 404, "store not found", err)
			return
		}
		baseWhere += " AND m.store_id = ?"
		args = append(args, store.ID)
	}

	// =========================
	// SEARCH
	// =========================
	if q != "" {
		like := "%" + q + "%"

		baseWhere += `
			AND (
				m.name LIKE ?
				OR m.phone LIKE ?
				OR sp.store_name LIKE ?
			)
		`

		args = append(args, like, like, like)
	}

	// =========================
	// FILTER TRANSACTION DATE
	// =========================
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	currentMonth := int(now.Month())
	currentYear := now.Year()

	month := currentMonth
	year := currentYear

	// validasi month
	if monthParam != "" {
		m, err := strconv.Atoi(monthParam)
		if err == nil && m >= 1 && m <= 12 {
			month = m
		}
	}
	// validasi year
	if yearParam != "" {
		y, err := strconv.Atoi(yearParam)
		if err == nil {
			year = y
		}
	}
	// awal bulan WIB
	startDate := time.Date(
		year,
		time.Month(month),
		1,
		0, 0, 0, 0,
		loc,
	)
	// akhir bulan WIB
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

	transactionWhere := fmt.Sprintf(`
		t.member_id = m.id
		AND t.status = 'done'
		AND t.created_at BETWEEN '%s' AND '%s'
	`, startDate.UTC(), endDate.UTC())

	// =========================
	// SORTING
	// =========================
	orderBy := "m.created_at DESC"

	switch sortBy {
	case "name":
		orderBy = fmt.Sprintf("m.name %s", sortType)
	case "store":
		orderBy = fmt.Sprintf("sp.store_name %s", sortType)
	case "total_point":
		orderBy = fmt.Sprintf("m.total_point %s", sortType)
	case "monthly_point":
		orderBy = fmt.Sprintf("monthly_point %s", sortType)
	case "transaction":
		orderBy = fmt.Sprintf("monthly_transaction %s", sortType)
	case "shopping":
		orderBy = fmt.Sprintf("total_shopping %s", sortType)
	}

	// =========================
	// COUNT
	// =========================
	var total int64

	countSQL := fmt.Sprintf(`
		SELECT COUNT(DISTINCT m.id)
		FROM members m
		LEFT JOIN store_profiles sp ON sp.id = m.store_id
		%s
	`, baseWhere)

	if len(args) > 0 { 
		if err := config.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	} else {
		if err := config.DB.Raw(countSQL).Scan(&total).Error; err != nil { 
			helpers.ErrorResponse(c, 500, "Failed to count members", err); 
			return 
		}
	}

	// =========================
	// RESPONSE STRUCT
	// =========================
	type Row struct {
		ID        uint    `json:"id"`
		Code      string  `json:"code"`
		Name      string  `json:"name"`
		Phone     string  `json:"phone"`
		StoreID   uint    `json:"store_id"`
		StoreName string  `json:"store_name"`

		MonthlyTransaction uint64   `json:"monthly_transaction"`
		MonthlyPoint     float64 `json:"monthly_point"`
		TotalShopping    float64 `json:"total_shopping"`
		TotalPoint       uint64  `json:"total_point"`
	}

	var rows []Row

	// =========================
	// DATA QUERY
	// =========================
	dataSQL := fmt.Sprintf(`
		SELECT
			m.id,
			m.code,
			m.name,
			m.phone,
			m.store_id,
			COALESCE(sp.store_name, '') AS store_name,

			COALESCE((
				SELECT COUNT(*)
				FROM transactions t
				WHERE %s
			), 0) AS monthly_transaction,

			COALESCE((
				SELECT SUM(t.member_point)
				FROM transactions t
				WHERE %s
			), 0) AS monthly_point,

			COALESCE((
				SELECT SUM(t.total_amount)
				FROM transactions t
				WHERE %s
			), 0) AS total_shopping,

			m.total_point
		FROM members m
		LEFT JOIN store_profiles sp ON sp.id = m.store_id

		%s

		ORDER BY %s
		LIMIT ? OFFSET ?
	`, transactionWhere, transactionWhere, transactionWhere, baseWhere, orderBy)

	queryArgs := append(args, limit, offset)

	if err := config.DB.Raw(dataSQL, queryArgs...).Scan(&rows).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to fetch members", err)
		return
	}

	lastPage := int(math.Ceil(float64(total) / float64(limit)))
	pagination := helpers.BuildPaginationLinks(
		c,
		page,
		limit,
		lastPage,
		len(rows),
		int(total),
	)

	c.JSON(http.StatusOK, response.Success("Members", gin.H{
		"data":    rows,
		"pagination": pagination,
	}))
}
func SummaryMember(c *gin.Context){
	monthParam := strings.TrimSpace(c.DefaultQuery("month", ""))
	yearParam := strings.TrimSpace(c.DefaultQuery("year", ""))
	storeParam := strings.TrimSpace(c.DefaultQuery("store_id", ""))

	// =========================
	// TOTAL MEMBER
	// =========================
	memberQuery := config.DB.Model(&models.Member{}).
		Where("deleted_at IS NULL")

	if storeParam != "" {
		storeID, err := strconv.Atoi(storeParam)
		if err != nil {
			helpers.ErrorResponse(c, 400, "invalid store_id", err)
			return
		}

		memberQuery = memberQuery.Where("store_id = ?", storeID)
	}

	var totalAll int64
	if err := memberQuery.Count(&totalAll).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count members", err)
		return
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	month := int(now.Month())
	year := now.Year()
	if monthParam != "" {
		m, e := strconv.Atoi(monthParam)
		if e == nil && m >= 1 && m <= 12 {
			month = m
		}
	}
	if yearParam != "" {
		y, e := strconv.Atoi(yearParam)
		if e == nil && y > 0 {
			year = y
		}
	}
	// awal bulan WIB
	startDate := time.Date(
		year,
		time.Month(month),
		1,
		0, 0, 0, 0,
		loc,
	)

	// akhir bulan WIB
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

	// =========================
	// TOTAL ACTIVE MEMBER
	// =========================
	activeSQL := `
		SELECT COUNT(DISTINCT t.member_id)
		FROM transactions t
		INNER JOIN members m ON m.id = t.member_id
		WHERE 
			t.status = 'done'
			AND m.deleted_at IS NULL
			AND t.created_at BETWEEN ? AND ?
	`
	activeArgs := []interface{}{
		startDate.UTC(),
		endDate.UTC(),
	}

	if storeParam != "" {
		storeID, _ := strconv.Atoi(storeParam)
		activeSQL += " AND m.store_id = ?"
		activeArgs = append(activeArgs, storeID)
	}

	var totalActive int64
	if err := config.DB.Raw(activeSQL, activeArgs...).Scan(&totalActive).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count active members", err)
		return
	}

	// =========================
	// NEW MEMBER
	// =========================
	newMemberQuery := config.DB.Model(&models.Member{}).
		Where(`
			deleted_at IS NULL
			AND created_at BETWEEN ? AND ?
		`, startDate.UTC(), endDate.UTC())

	if storeParam != "" {
		storeID, _ := strconv.Atoi(storeParam)
		newMemberQuery = newMemberQuery.Where("store_id = ?", storeID)
	}

	var newMember int64
	if err := newMemberQuery.Count(&newMember).Error; err != nil {
		helpers.ErrorResponse(c, 500, "failed to count new members", err)
		return
	}

	// =========================
	// INACTIVE MEMBER
	// =========================
	totalInactive := totalAll - totalActive

	c.JSON(http.StatusOK, response.Success("Member summary", gin.H{
		"total_all": totalAll,
		"total_active": totalActive,
		"total_inactive": totalInactive,
		"new_member": newMember,
	}))
}
func DetailMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil {
		helpers.ErrorResponse(c, 400, "Invalid id", err);
		return
	}

    var m models.Member
    if err := config.DB.First(&m, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Member not found", err);
		return
	}
    c.JSON(http.StatusOK, response.Success("Detail Member", m))
}
func CreateMember(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User)

	type payload struct {
        Name string  `json:"name" binding:"required"`
		Phone string `json:"phone" binding:"required"`
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
			case "Name":
				if e.Tag() == "required" {
					errorsMap["name"] = "Nama wajib diisi"
				}
			case "Phone":
				if e.Tag() == "required" {
					errorsMap["phone"] = "Telepon wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}
	
	var otherM models.Member
	if err := config.DB.Model(&models.Member{}).
		Where("phone = ? AND store_id = ?", p.Phone, *user.StoreID).
		First(&otherM).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 500, "Failed query", err)
			return
		}
	}
	// cek field mana yang duplicate
	if otherM.Phone == p.Phone {
		helpers.ErrorResponse(c, 422, "Phone already in use", nil)
		return
	}

	code := helpers.RandomString(5)
    member := models.Member{
		StoreID: user.StoreID,
		Code:    code,
		Name:  p.Name,
		Phone: p.Phone,
	}

    if err := config.DB.Create(&member).Error; err != nil { 
		helpers.ErrorResponse(c, 500, "Failed to create member", err); 
		return 
	}

    c.JSON(http.StatusOK, response.Success("Member created", member))
}
func AdminCreateMember(c *gin.Context) {
	type payload struct {
		StoreID	uint64	`json:"store_id" binding:"required"`
        Name string  `json:"name" binding:"required"`
		Phone string `json:"phone" binding:"required"`
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
			case "StoreID":
				if e.Tag() == "required" {
					errorsMap["store_id"] = "Store ID wajib diisi"
				}
			case "Name":
				if e.Tag() == "required" {
					errorsMap["name"] = "Nama wajib diisi"
				}
			case "Phone":
				if e.Tag() == "required" {
					errorsMap["phone"] = "Telepon wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}
	
	var otherM models.Member
	if err := config.DB.Model(&models.Member{}).
		Where("phone = ?", p.Phone).
		First(&otherM).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 500, "Failed query", err)
			return
		}
	}
	// cek field mana yang duplicate
	if otherM.Phone == p.Phone {
		helpers.ErrorResponse(c, 422, "Phone already in use", nil)
		return
	}

	code := helpers.RandomString(5)
    member := models.Member{
		StoreID: &p.StoreID,
		Code:    code,
		Name:  p.Name,
		Phone: p.Phone,
	}

    if err := config.DB.Create(&member).Error; err != nil { 
		helpers.ErrorResponse(c, 500, "Failed to create member", err); 
		return 
	}

    c.JSON(http.StatusOK, response.Success("Member created", member))
}
func UpdateMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil { 
		helpers.ErrorResponse(c, 400, "Invalid id", err); 
		return 
	}

	var p struct {
        Name string  `json:"name" binding:"required"`
		Phone string `json:"phone" binding:"required"`
    }
    if err := c.ShouldBindJSON(&p); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Format JSON tidak valid"})
			return
		}
		errorsMap := make(map[string]string)
		for _, e := range ve {
			switch e.Field() {
			case "Name":
				if e.Tag() == "required" {
					errorsMap["name"] = "Nama wajib diisi"
				}
			case "Phone":
				if e.Tag() == "required" {
					errorsMap["phone"] = "Telepon wajib diisi"
				}
			default:
				errorsMap[e.Field()] = "Validasi gagal"
			}
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Validasi gagal", "errors": errorsMap})
		return
	}
	
    var m models.Member
    if err := config.DB.First(&m, id).Error; err != nil { 
		helpers.ErrorResponse(c, 404, "Member not found", err); return 
	}

	var otherM models.Member
	if err := config.DB.Model(&models.Member{}).
		Where("id != ?", id).
		Where("phone = ?", p.Phone).
		First(&otherM).Error; err != nil {
		
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			helpers.ErrorResponse(c, 500, "Failed query", err)
			return
		}
	}
	// cek field mana yang duplicate
	if otherM.Phone == p.Phone {
		helpers.ErrorResponse(c, 422, "Phone already in use", nil)
		return
	}

	if err := config.DB.Model(&m).Updates(models.Member{
		Name:  p.Name,
		Phone: p.Phone,
	}).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to update member", err); 
		return
	}

    c.JSON(http.StatusOK, response.Success("Member updated", m))
}
func DeleteMember(c *gin.Context) {
    idParam := c.Param("id")
    id, err := strconv.ParseUint(idParam, 10, 64)
    if err != nil { 
		helpers.ErrorResponse(c, 400, "Invalid id", err); 
		return 
	}
	var m models.Member
	if err := config.DB.First(&m, id).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Member not found", err);
		return
	}
	if err := config.DB.Delete(&m).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Failed to delete member", err);
		return
	}

	c.JSON(http.StatusOK, response.Success("Member deleted", nil))
}
