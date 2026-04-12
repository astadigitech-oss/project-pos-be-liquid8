package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"liquid8/pos/config"
	"liquid8/pos/helpers"
	"liquid8/pos/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SeederConfig struct {
	Table string
	Run   func() error
}

var seederRegistry = map[string]SeederConfig{
	"store": {
		Table: "store_profiles",
		Run:   func() error { return seedStore(config.DB) },
	},
	"product": {
		Table: "products",
		Run:   func() error { return seedProduct(config.DB) },
	},
	"user": {
		Table: "users",
		Run:   func() error { return seedUsers(config.DB) },
	},
	"ppn": {
		Table: "ppns",
		Run:   func() error { return seedPPN(config.DB) },
	},
}

var seederOrder = []string{
	"store",
	"product",
	"user",
	"ppn",
}

func main() {
	app_env := os.Getenv("APP_ENV")
	if app_env == "production" {
		log.Fatal("❌ Gagal: sistem saat ini dalam mode production")
	}

	// Cek argumen
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/seeder/main.go --all 		(untuk seed semua table)")
		fmt.Println("  go run ./cmd/seeder/main.go --class=name 	(untuk seed table tertentu)")
		fmt.Println("  go run ./cmd/seeder/main.go --list 		(untuk melihat list name class)")
		return
	}

	command := os.Args[1]

	if command == "--all"{
		config.InitDB()
		runAllSeeders()
		return

	}else if strings.HasPrefix(command, "--class=") {
		parts := strings.SplitN(command, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			log.Fatal("❌ Format salah. Gunakan --class=namaSeeder")
		}
		className := parts[1]
		config.InitDB()
		runSingleSeeder(className)
		return

	}else if command == "--list" {
		fmt.Println("📦 Available Seeder Classes:")
		fmt.Println("--------------------------------")
		fmt.Printf("  %-18s %s\n", "[class_name]", "[table_name]")
		for name, config := range seederRegistry {
			fmt.Printf("  - %-15s → %s\n", name, config.Table)
		}
		fmt.Println("--------------------------------")
		fmt.Println("Usage: go run ./cmd/seeder/main.go --class=[class_name]")

		return
	}else{
		fmt.Printf("Perintah '%s' tidak dikenali.\n", command)
		fmt.Println("Gunakan -all atau -class")
	}
}

func runAllSeeders() {
	if err := truncateTables(); err != nil  {
		log.Fatal("❌ Gagal :", err)
	}

	log.Println("🚀 Menjalankan semua seeder...")

	for _, name := range seederOrder {
		config, exists := seederRegistry[name]
		if !exists {
			log.Fatalf("Seeder %s tidak ditemukan", name)
		}
		
		if err := config.Run(); err != nil {
			log.Printf("→ %s ❌\n", name)
			log.Fatal("❌ Gagal:", err)
		}
		log.Printf("→ %s ✅\n", name)
	}

	log.Println("✅ Semua seeder selesai")
}

func runSingleSeeder(name string) {
	config, exists := seederRegistry[name]
	if !exists {
		log.Fatalf("❌ Seeder '%s' tidak ditemukan", name)
	}

	if err := truncateTableByClass(name); err != nil  {
		log.Fatal(err.Error())
	}

	log.Printf("🚀 Menjalankan seeder: %s\n", name)

	if err := config.Run(); err != nil {
		log.Fatal("❌ Gagal:", err)
	}

	log.Println("✅ Seeder selesai")
}

//==================================================================
// Seeder
//==================================================================
func seedStore(db *gorm.DB) error {
	stores := []models.StoreProfile{
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Proklamasi",
			Phone:     "08",
			Address:   "Diskonter Proklamasi",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Pinang",
			Phone:     "08",
			Address:   "Diskonter Pinang",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Cinere",
			Phone:     "08",
			Address:   "Diskonter Cinere",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Kayu Manis",
			Phone:     "08",
			Address:   "Diskonter Kayu Manis",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Zambrud",
			Phone:     "08",
			Address:   "Diskonter Zambrud",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Bintaro",
			Phone:     "08",
			Address:   "Diskonter Bintaro",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Pekayon",
			Phone:     "08",
			Address:   "Diskonter Pekayon",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Harapan",
			Phone:     "08",
			Address:   "Diskonter Harapan",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Loji",
			Phone:     "08",
			Address:   "Diskonter Loji",
		},
		{
			Token:     helpers.RandomString(25),
			StoreName: "Diskonter Mayor Oking",
			Phone:     "08",
			Address:   "Diskonter Mayor Oking",
		},
	}

	return db.Create(&stores).Error
}
func seedUsers(db *gorm.DB) error {
	password, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

	var users []models.User
	users = append(users, models.User{
		Name: "Administrator",
		Username: "admin",
		Password: string(password),
		Email: "admin@gmail.com",
		Role: "admin",
	})
	storeIDs := []uint64{1,2,3,4,5,6,7,8,9,10}
	for _,store_id := range storeIDs {
		users = append(users, models.User{
			Name: fmt.Sprintf("kasir%d",store_id), 
			StoreID: &store_id, 
			Username: fmt.Sprintf("kasir%d",store_id), 
			Email: fmt.Sprintf("kasir%d@gmail.com", store_id), 
			Password: string(password), 
			Role: "kasir",
		})
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoNothing: true,
	}).Create(&users).Error
}
func seedProduct(db *gorm.DB) error {
	// seed 100 dummy products
	var products []models.Product
	for j := 1; j <= 10; j++ {
		for i := 1; i <= 50; i++ {
			// distribute stores 1..10
			storeID := uint64(j)
			barcode := fmt.Sprintf("P%d%06d", j, i)
			price := float64(10000 + (i * 250))
	
			p := models.Product{
				StoreID:     storeID,
				Barcode:     barcode,
				Name:        fmt.Sprintf("Product %d", i),
				Price:       price,
				Quantity:    int64((i%20)+1),
				Status:      "display",
				TagColor:    "blue",
				OldPrice:    price,
				ActualPrice: price,
			}
			products = append(products, p)
		}
	}

	// insert with do-nothing on conflict so seeder is idempotent
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "barcode"}},
		DoNothing: true,
	}).Create(&products).Error
}
func seedPPN(db *gorm.DB) error {
	ppns := models.Ppn{
		Ppn:         11,
		IsTaxDefault: true,
	}

	return db.Create(&ppns).Error
}
func truncateTableByClass(className string) error {

	conf, exists := seederRegistry[className]
	if !exists {
		return fmt.Errorf("seeder '%s' tidak memiliki mapping table", className)
	}

	// Matikan FK
	if err := config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return err
	}

	if err := config.DB.Exec("TRUNCATE TABLE " + conf.Table).Error; err != nil {
		config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
		return err
	}

	// Hidupkan FK
	return config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
}
func truncateTables() error {
	// tables := []string{
	// 	"roles",
	// 	"users",
	// 	"color_tags",
	// 	"categories",
	// 	"racks",
	// 	"loyalty_ranks",
	// 	"buyers",
	// }
	// Matikan foreign key check
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 0")

	for _, class := range seederOrder {
		conf, exists := seederRegistry[class]
		if !exists {
			log.Fatalf("❌ Seeder '%s' tidak ditemukan", class)
		}

		if err := config.DB.Exec("TRUNCATE TABLE " + conf.Table).Error; err != nil {
			config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
			return err
		}
	}

	// Hidupkan lagi
	config.DB.Exec("SET FOREIGN_KEY_CHECKS = 1")
	return nil
}

