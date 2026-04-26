package controllers

import (
	"fmt"
	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type uploadReleaseRequest struct {
	Version string `form:"version" binding:"required"`
	Notes   *string `form:"notes"`

	WindowsSignature string `form:"windows_signature" binding:"required"`
	MacSignature     string `form:"mac_signature" binding:"required"`
}

func UploadRelease(c *gin.Context) {
	var req uploadReleaseRequest

	if err := c.ShouldBind(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Payload tidak valid",
			})
			return
		}

		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "version":
				errors["version"] = "Version wajib diisi"
			case "windowssignature":
				errors["windows_signature"] = "Windows Signature wajib diisi"
			case "macsignature":
				errors["mac_signature"] = "Mac Signature wajib diisi"
			default:
				errors[field] = "Field ini tidak valid"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	// ambil file
	winInstaller, _ := c.FormFile("windows_installer_updater")
	macInstaller, _ := c.FormFile("mac_installer")
	macUpdater, _ := c.FormFile("mac_updater")

	// VALIDASI FILE WAJIB
	if winInstaller == nil {
		helpers.ErrorResponse(c, 422, "File Windows installer / updater wajib diinput", nil)
		return
	}
	if macInstaller == nil {
		helpers.ErrorResponse(c, 422, "File Mac installer wajib diinput", nil)
		return
	}
	if macUpdater == nil {
		helpers.ErrorResponse(c, 422, "File Mac Updater wajib diinput", nil)
		return
	}

	// VALIDASI EXTENSION
	if err := validateFile(winInstaller, []string{".exe"}); err != nil {
		helpers.ErrorResponse(c, 422, "File installer harus .exe", nil)
		return
	}
	if err := validateFile(macInstaller, []string{".dmg"}); err != nil {
		helpers.ErrorResponse(c, 422, "File installer harus .dmg", nil)
		return
	}
	if err := validateTarGz(macUpdater); err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}

	// simpan file
	winInstallerName, err := saveFile(c, winInstaller, "./public/installers/windows")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal menyimpan file", err)
		return
	}
	macInstallerName, err := saveFile(c, macInstaller, "./public/installers/mac")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal menyimpan file", err)
		return
	}
	macUpdaterName, err := saveFile(c, macUpdater, "./public/updaters/mac")
	if err != nil {
		helpers.ErrorResponse(c, 500, "Gagal menyimpan file", err)
		return
	}

	// simpan ke DB
	data := models.AppRelease{
		Version: req.Version,
		Notes:   req.Notes,

		WindowsInstaller: winInstallerName,
		WindowsUpdater: winInstallerName,
		MacInstaller:     macInstallerName,
		MacUpdater:       macUpdaterName,

		WindowsSignature: req.WindowsSignature,
		MacSignature:     req.MacSignature,
	}

	if err := config.DB.Create(&data).Error; err != nil {
		helpers.ErrorResponse(c, 500, "Gagal upload app release", err)
		return
	}

	c.JSON(200, gin.H{
		"message": "Upload berhasil",
		"data":    data,
	})
}
func DownloadInstaller(c *gin.Context) {
	target := c.Param("target")
	fileName := c.Param("file")

	if target != "windows" && target != "darwin" {
		helpers.ErrorResponse(c, 422, "Target harus windows atau darwin", nil)
		return
	}

	// var release models.AppRelease
	// if err := config.DB.Order("created_at DESC").First(&release).Error; err != nil {
	// 	helpers.ErrorResponse(c, 500, "Interal server error", err)
	// 	return
	// }

	var filePath string

	if target == "windows" {
		filePath = "./public/installers/windows/" + fileName
	} else {
		filePath = "./public/installers/mac/" + fileName
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		helpers.ErrorResponse(c, 404, fmt.Sprintf("File %s tidak ditemukan", fileName), nil)
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.File(filePath)
}
func CheckUpdate(c *gin.Context) {
	target := c.Param("target")

	var release models.AppRelease

	if err := config.DB.Order("created_at DESC").First(&release).Error; err != nil {
		helpers.ErrorResponse(c, 404, "Tidak ada data", nil)
		return
	}

	var url_installer, url_updater, signature string

	switch target {
	case "windows":
		url_installer = fmt.Sprintf("%s/api/installer/windows/file/%s", os.Getenv("APP_URL"), release.WindowsInstaller)
		url_updater = fmt.Sprintf("%s/api/update/windows/file/%s", os.Getenv("APP_URL"), release.WindowsUpdater)
		signature = release.WindowsSignature
	case "darwin":
		url_installer = fmt.Sprintf("%s/api/installer/darwin/file/%s", os.Getenv("APP_URL"), release.MacInstaller)
		url_updater = fmt.Sprintf("%s/api/update/darwin/file/%s", os.Getenv("APP_URL"), release.MacUpdater)
		signature = release.MacSignature
	default:
		helpers.ErrorResponse(c, 400, "Target tidak valid, harus windows atau darwin", nil)
		return
	}

	c.JSON(200, gin.H{
		"version":   release.Version,
		"pub_date":  helpers.ToLocalTime(release.CreatedAt, "Asia/Jakarta"),
		"url_installer":       url_installer,
		"url_updater":       url_updater,
		"signature": signature,
		"notes":     release.Notes,
	})
}
func DownloadUpdater(c *gin.Context) {
	target := c.Param("target")
	fileName := c.Param("file")

	if target != "windows" && target != "darwin" {
		helpers.ErrorResponse(c, 422, "Target harus windows atau darwin", nil)
		return
	}

	// var release models.AppRelease
	// if err := config.DB.Order("created_at DESC").First(&release).Error; err != nil {
	// 	helpers.ErrorResponse(c, 500, "Interal server error", err)
	// 	return
	// }

	var filePath string

	if target == "windows" {
		filePath = "./public/installers/windows/" + fileName
	} else {
		filePath = "./public/updaters/mac/" + fileName
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		helpers.ErrorResponse(c, 404, fmt.Sprintf("File %s tidak ditemukan", fileName), nil)
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.File(filePath)
}

// ========================= helper ==========================
func saveFile(c *gin.Context, file *multipart.FileHeader, path string) (string, error) {
	filename := fmt.Sprintf("%d_%s", time.Now().Unix(), file.Filename)
	fullPath := filepath.Join(path, filename)

	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		return "", err
	}

	return filename, nil
}

func validateFile(file *multipart.FileHeader, allowedExt []string) error {
	ext := strings.ToLower(filepath.Ext(file.Filename))

	for _, v := range allowedExt {
		if ext == v {
			return nil
		}
	}

	return fmt.Errorf("file %s tidak diizinkan, hanya boleh: %v", file.Filename, allowedExt)
}

func validateTarGz(file *multipart.FileHeader) error {
	name := strings.ToLower(file.Filename)

	if strings.HasSuffix(name, ".tar.gz") {
		return nil
	}

	return fmt.Errorf("file harus format .tar.gz")
}