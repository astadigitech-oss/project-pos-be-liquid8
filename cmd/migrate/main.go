package main

import (
	"fmt"
	"liquid8/pos/config"
	"liquid8/pos/models"
	"os"

	"log"
)

func main() {
	// Cek Argumen
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/migrate/main.go -up     (Untuk migrasi tabel)")
		fmt.Println("  go run ./cmd/migrate/main.go -fresh  (Untuk empty table)")
		fmt.Println("  go run ./cmd/migrate/main.go -drop   (Untuk menghapus semua tabel)")
		return
	}

	command := os.Args[1]

	switch command {
	case "-up":
		config.InitDB()
		runMigrations()
	case "-drop":
		config.InitDB()
		dropMigrations()
	case "-fresh":
		config.InitDB()
		truncateAllTables(os.Getenv("DB_NAME"))
	default:
		fmt.Printf("Perintah '%s' tidak dikenali.\n", command)
		fmt.Println("Gunakan -up atau -drop")
	}
}

var Tables = []interface{}{
	&models.StoreProfile{},
	&models.Member{},
	&models.User{},
	&models.UserToken{},
	&models.MigrateProductHistory{},
	&models.Product{},
	&models.Shift{},
	&models.Transaction{},
	&models.TransactionItem{},
	&models.CartItem{},
	&models.Ppn{},
}

func runMigrations() {
	log.Println("⏳ Menjalankan migrasi...")
	err := config.DB.AutoMigrate(Tables...)

	if err != nil {
		log.Fatal("❌ Gagal migrate:", err)
	}

	log.Println("✅ Migrasi berhasil, semua tabel siap!")
}

func dropMigrations() {
	log.Println("⚠️ Menghapus semua tabel...")
	// Matikan pengecekan foreign key
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0;")
	err := config.DB.Migrator().DropTable(Tables...)
	// Hidupkan kembali pengecekan foreign key
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1;")

	if err != nil {
		log.Fatalf("Gagal Drop Tabel: %v", err)
	}
	log.Println("Tabel Berhasil Dihapus!")
}

func truncateAllTables(dbName string) {
	log.Println("⏳ Truncate semua tabel...")
	var tables []string

	err := config.DB.Raw(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = ?
	`, dbName).Scan(&tables).Error
	if err != nil {
		log.Fatal("❌ Empty Error :", err)
	}

	// Matikan foreign key check
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")

	for _, table := range tables {
		config.DB.Exec("TRUNCATE TABLE " + table)
	}

	// Hidupkan lagi
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")

	log.Println("✅ Semua tabel berhasil dikosongkan!")
	// return nil
}