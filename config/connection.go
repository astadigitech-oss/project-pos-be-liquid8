package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	exe, _ := os.Executable()
	exePath := filepath.Dir(exe)
	err := godotenv.Load(filepath.Join(exePath, ".env"))
	if err != nil {
		// coba load dari Current Working Directory
		err = godotenv.Load() 
		if err != nil {
			log.Println("⚠️ .env tidak ditemukan, menggunakan environment system")
		}
	}

	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASS")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")

	if user == "" || host == "" || port == "" || name == "" {
		log.Fatal("❌ Config database tidak lengkap (cek ENV)")
	}

	//koneksi database mysql => {user}:{pass}@tcp({host}:{port})/{database}
	query := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC", user, pass, host, port, name)

	db, err := gorm.Open(mysql.Open(query), &gorm.Config{})
	if err != nil {
		log.Fatal("❌ Gagal koneksi ke database: ", err)
	}

	DB = db
	fmt.Println("✅ Database terkoneksi")

}
