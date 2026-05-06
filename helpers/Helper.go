package helpers

import (
	"database/sql"
	"errors"
	"liquid8/pos/models"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"fmt"
	mrand "math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)


func ParseIntOrDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		// try float then cast
		var f float64
		_, err2 := fmt.Sscanf(s, "%f", &f)
		if err2 != nil {
			return def
		}
		return int(f)
	}
	return i
}
func StringToInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
func ParseFloatOrDefault(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil {
		return def
	}
	return f
}
func Float64Ptr(v float64) *float64 {
	return &v
}
func IntPtr(v int) *int {
	return &v
}
func GetFloat64Value(ptr *float64) float64 {
    if ptr != nil {
        return *ptr
    }
    return 0.0
}
func ToStringNumber(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}
func RandomString(n int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstupwxyz0123456789@#$"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		ret[i] = letters[mrand.Intn(len(letters))]
	}
	return string(ret)
}
func NormalizeRackName(rackname string) string {
	rackName := strings.ToUpper(strings.TrimSpace(rackname))
	// ambil string setelah tanda "-"
	if idx := strings.Index(rackName, "-"); idx != -1 {
		rackName = rackName[idx+1:]
	}
	// ganti sisa "-" jadi spasi
	rackName = strings.ReplaceAll(rackName, "-", " ")
	re := regexp.MustCompile(`\s+\d+$`)
	rackName = re.ReplaceAllString(rackName, "")

	return rackName
}

// ========================= Custom Error =========================
type CustomError struct {
	StatusCode int
	Message    string
	Err        error 
}

func (e *CustomError) Error() string {
	if e.Err != nil {
		// Gabungkan pesan kustom dengan pesan error asli (jika ada)
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}
func NewCustomError(code int, msg string, err error) *CustomError {
	return &CustomError{
		StatusCode: code,
		Message:    msg,
		Err:        err,
	}
}
// ==================================================================

func GetToday() string {
	location,_ := time.LoadLocation("Asia/Jakarta")

	nowInJakarta := time.Now().In(location)

	// 3. Truncate waktu ke awal hari (00:00:00) di Jakarta
	startOfDayInJakarta := time.Date(
		nowInJakarta.Year(),
		nowInJakarta.Month(),
		nowInJakarta.Day(),
		0, 0, 0, 0,
		location,
	)

	return startOfDayInJakarta.Format("2006-01-02")
}
func GetCurentTime(tz string) (time.Time, error) {
	location, err := time.LoadLocation(tz)
	if err != nil {
		fmt.Println("Invalid timezone:", tz)
		return time.Time{}, err
	}

	nowInLocation := time.Now().In(location)
	return nowInLocation, nil
}
func ToLocalTime(t time.Time, tz string) time.Time {
	location, _ := time.LoadLocation(tz)
	return t.In(location)
}
func BuildPaginationLinks(
	c *gin.Context,
	currentPage, limit, lastPage, lenData, totalData int,
) gin.H {

	links := []gin.H{}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	baseURL := fmt.Sprintf(
		"%s://%s%s",
		scheme,
		c.Request.Host,
		c.Request.URL.Path,
	)

	existingQueries := c.Request.URL.Query()
	buildURL := func(page int) string {
		params := make(url.Values)
        for k, v := range existingQueries {
			params[k] = v
		}

        // Override atau Set parameter 'page'
        params.Set("page", strconv.Itoa(page))
        
        return baseURL + "?" + params.Encode()
	}

	// PREVIOUS
	links = append(links, gin.H{
		"url":    ternary(currentPage > 1, buildURL(currentPage-1), nil),
		"label":  "&laquo; Previous",
		"active": false,
	})

	window := 2
	start := max(2, currentPage-window)
	end := min(lastPage-1, currentPage+window)

	// FIRST PAGE
	links = append(links, gin.H{
		"url":    buildURL(1),
		"label":  "1",
		"active": currentPage == 1,
	})

	// LEFT DOTS
	if start > 2 {
		links = append(links, gin.H{
			"url": nil,
			"label": "...",
			"active": false,
		})
	}

	// MIDDLE
	for i := start; i <= end; i++ {
		links = append(links, gin.H{
			"url":    buildURL(i),
			"label":  strconv.Itoa(i),
			"active": i == currentPage,
		})
	}

	// RIGHT DOTS
	if end < lastPage-1 {
		links = append(links, gin.H{
			"url": nil,
			"label": "...",
			"active": false,
		})
	}

	// LAST PAGE
	if lastPage > 1 {
		links = append(links, gin.H{
			"url":    buildURL(lastPage),
			"label":  strconv.Itoa(lastPage),
			"active": currentPage == lastPage,
		})
	}

	// NEXT
	links = append(links, gin.H{
		"url":    ternary(currentPage < lastPage, buildURL(currentPage+1), nil),
		"label":  "Next &raquo;",
		"active": false,
	})

	offset := (currentPage - 1) * limit
	var from, to *int
	if lenData > 0 {
		from = IntPtr(offset + 1)
		to = IntPtr(offset + lenData)
	}
	return gin.H{
		"current_page":   currentPage,
		"from":           from,
		"last_page":      lastPage,
		// "links":          links,
		"per_page":       limit,
		"to":             to,
		"total":          totalData,
	}
}
func ternary(condition bool, a, b interface{}) interface{} {
	if condition {
		return a
	}
	return b
}

func HumanizeNumber(n float64) string {
    s := fmt.Sprintf("%.0f", n)
    nStr := ""
    for i, c := range reverse(s) {
        if i != 0 && i%3 == 0 {
            nStr = "." + nStr
        }
        nStr = string(c) + nStr
    }
    return nStr
}

func reverse(s string) string {
    r := []rune(s)
    for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
        r[i], r[j] = r[j], r[i]
    }
    return string(r)
}

// ========================== Helper logger =====================
var Errlog = NewLogger("./logs/errors.log")

func NewLogger(path string) *logrus.Logger {
	log := logrus.New()

	// pastikan folder ada
	os.MkdirAll(filepath.Dir(path), os.ModePerm)

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		panic(err)
	}

	log.SetOutput(file)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetLevel(logrus.InfoLevel)

	return log
}

// Helper untuk cek memory usage
func GetMemoryUsageMB() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}
func FlexibleNormalize(input string) string {
	s := strings.TrimSpace(input)

	// regex untuk detect pure currency/number
	// contoh yang match:
	// Rp14,000
	// 10,000
	// 14000
	currencyPattern := regexp.MustCompile(`^(?i)\s*rp?\s?[\d.,]+\s*$`)

	if currencyPattern.MatchString(s) {
		// hapus Rp (case insensitive)
		reRp := regexp.MustCompile(`(?i)rp`)
		s = reRp.ReplaceAllString(s, "")

		// hapus spasi
		s = strings.TrimSpace(s)

		// hapus separator ribuan
		s = strings.ReplaceAll(s, ",", "")
		s = strings.ReplaceAll(s, ".", "")

		return s
	}

	// kalau bukan pure angka/currency → biarkan
	return s
}
func ParseFlexibleDate(input string, tz string) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, err
	}

    layouts := []string{
        "2006-01-02", // standar API
        "02-01-2006", // format indo
        "02/01/2006", // alternatif
    }

    for _, layout := range layouts {
        if t, err := time.ParseInLocation(layout, input, loc); err == nil {
            return t, nil
        }
    }

     return time.Time{}, fmt.Errorf(
        "format tanggal '%s' tidak dikenali. Gunakan format: YYYY-MM-DD atau DD-MM-YYYY",
        input,
    )
}
func ErrorResponse(c *gin.Context, status int, message string, err error) {
	response := gin.H{
		"success": false,
		"message": message,
	}

	if err != nil {
		Errlog.WithFields(logrus.Fields{
		"path":   c.Request.URL.Path,
		"method": c.Request.Method,
	}).WithError(err).Error(fmt.Sprintf("%s", message))

		if os.Getenv("APP_ENV") == "development" {
			response["error"] = err.Error()
		}
	}

	c.JSON(status, response)
}

//build style excel
func BuildStyle(f *excelize.File, styles ...excelize.Style) (int, error) {
	merged := MergeStyles(styles...)
	return f.NewStyle(merged)
}
func MergeStyles(styles ...excelize.Style) *excelize.Style {
	result := &excelize.Style{}

	for _, s := range styles {

		if s.Font != nil {
			result.Font = s.Font
		}

		if s.Alignment != nil {
			result.Alignment = s.Alignment
		}

		if s.Border != nil {
			result.Border = append(result.Border, s.Border...)
		}

		if s.Fill.Type != "" {
			result.Fill = s.Fill
		}

		if s.NumFmt != 0 {
			result.NumFmt = s.NumFmt
		}
	}

	return result
}

//=================== Transaction ========================
func RoundTo500(n int) float64 {
	remainder := n % 1000

	if remainder == 0 {
		return float64(n)
	}

	if remainder <= 500 {
		return float64((n - remainder) + 500)
	}

	return float64((n - remainder) + 1000)
}
func RecalculateTransactionShift(db *gorm.DB, storeID uint64, shiftID uint64) (map[string]interface{}, error) {
	type result struct {
		Subtotal   float64
		TotalInvoice int64 
		TotalAmount float64  //total subtotal + pajak
		TotalRounded float64  //total pembulatan
		TaxAmount  float64	 //total pajak
		
		Cash       float64
		Transfer   float64
		Qris       float64

		CashCancelled float64
		TransferCancelled float64
		QrisCancelled float64
	}

	var res result

	err := db.Model(&models.Transaction{}).
		Select(`
			COUNT(*) as total_invoice,
			COALESCE(SUM(CASE WHEN status = 'done' THEN subtotal ELSE 0 END), 0) AS subtotal,
			COALESCE(SUM(CASE WHEN status = 'done' THEN total_amount ELSE 0 END), 0) AS total_amount,
			COALESCE(SUM(CASE WHEN status = 'done' THEN tax_price ELSE 0 END), 0) AS tax_amount,
			COALESCE(SUM(CASE WHEN status = 'done' THEN rounded_price ELSE 0 END),0) AS total_rounded,
			
			COALESCE(SUM(CASE WHEN payment_method = 'cash' AND status = 'done' THEN total_amount ELSE 0 END),0) AS cash,
			COALESCE(SUM(CASE WHEN payment_method = 'transfer' AND status = 'done' THEN total_amount ELSE 0 END),0) AS transfer,
			COALESCE(SUM(CASE WHEN payment_method = 'qris' AND status = 'done' THEN total_amount ELSE 0 END),0) AS qris,

			COALESCE(SUM(CASE WHEN payment_method = 'cash' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS cash_cancelled,
			COALESCE(SUM(CASE WHEN payment_method = 'transfer' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS transfer_cancelled,
			COALESCE(SUM(CASE WHEN payment_method = 'qris' AND status = 'cancelled' THEN total_amount ELSE 0 END),0) AS qris_cancelled
		`).
		Where("shift_id = ?", shiftID).
		Where("store_id = ?", storeID).
		Scan(&res).Error

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_invoice":   res.TotalInvoice,
		"subtotal":   res.Subtotal,
		"total_amount": res.TotalAmount,
		"total_rounded": res.TotalRounded,
		"tax_amount":  res.TaxAmount,
		"cash":     res.Cash,
		"transfer": res.Transfer,
		"qris":     res.Qris,
		"total_cash_cancel":     res.CashCancelled,
		"total_transfer_cancel": res.TransferCancelled,
		"total_qris_cancel":     res.QrisCancelled,
	}, nil
}
func RecalculateCart(tx *gorm.DB, cartID uint64) error {
	var subtotal float64

	// hitung ulang dari cart_items
	if err := tx.Model(&models.CartItem{}).
		Where("cart_id = ?", cartID).
		Select("COALESCE(SUM(subtotal),0)").
		Scan(&subtotal).Error; err != nil {
		return err
	}

	// ambil data cart (untuk packaging nanti)
	var cart models.Cart
	if err := tx.First(&cart, cartID).Error; err != nil {
		return errors.New("cart tidak ditemukan: " + err.Error())
	}

	// base untuk ppn
	grand_total := subtotal

	// update cart
	return tx.Model(&models.Cart{}).
		Where("id = ?", cartID).
		Updates(map[string]interface{}{
			"subtotal":    subtotal,
			"grand_total": grand_total,
		}).Error
}
func GeneratePendingKeepCode(db *gorm.DB, storeID uint64) (string, error) {
	// MySQL: substring start index is 1-based. Prefix 'KSRPEND' length = 7, numeric part starts at 8
	var maxNum sql.NullInt64
	row := db.Raw(`
		SELECT COALESCE(MAX(CAST(SUBSTRING(keep_code, 8) AS UNSIGNED)), 0) as maxnum
		FROM carts
		WHERE keep_code IS NOT NULL AND store_id = ?
	`, storeID).Row()

	if err := row.Scan(&maxNum); err != nil {
		return "", err
	}

	next := int64(1)
	if maxNum.Valid && maxNum.Int64 > 0 {
		next = maxNum.Int64 + 1
	}

	code := fmt.Sprintf("KSRPEND%05d", next)
	return code, nil
}
func GenerateInvoice(db *gorm.DB, storeID uint64) (string, error) {
	//prefix format : INV-[storeID]LQ00001
	prefix := fmt.Sprintf("INV-%dLQ", storeID)
	var maxNum sql.NullInt64

	// Use transactions.invoice column and handle variable-length prefix
	// CHAR_LENGTH(?) + 1 is the start index for the numeric suffix
	row := db.Raw(`
		SELECT COALESCE(MAX(CAST(SUBSTRING(invoice, CHAR_LENGTH(?) + 1) AS UNSIGNED)), 0) AS maxnum
		FROM transactions
		WHERE invoice LIKE CONCAT(?, '%')
	`, prefix, prefix).Row()

	if err := row.Scan(&maxNum); err != nil {
		return "", err
	}

	next := int64(1)
	if maxNum.Valid && maxNum.Int64 > 0 {
		next = maxNum.Int64 + 1
	}

	code := fmt.Sprintf("%s%05d", prefix, next)
	return code, nil
}
func GetActiveShift(db *gorm.DB, storeID uint) (*models.Shift, error) {
	var shift models.Shift
	err := db.Where("store_id = ? AND status = ?", storeID, "open").
		First(&shift).Error

	if err != nil {
		return nil, err
	}

	return &shift, nil
}

func GetDayIndo(t time.Time) string {
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

