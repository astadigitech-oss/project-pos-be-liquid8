package models

import (
	"time"
	"fmt"
)

type Product struct {
	ID            		uint64      `gorm:"primaryKey;autoIncrement" json:"id"`
	StoreID  	  		uint64     `json:"store_id" gorm:"index;not null"`
	MigrateID  	  		*uint64     `json:"migrte_id" gorm:"index"`
	
	CodeDocument  		*string     `gorm:"size:15" json:"code_document"`
	OldBarcode		 	*string  	`gorm:"size:50" json:"old_barcode"`
	OldPrice		  	float64		`gorm:"type:decimal(18,2);not null" json:"old_price"`
	ActualPrice			float64		`gorm:"type:decimal(18,2);not null" json:"actual_price"`
	Barcode       		string     	`gorm:"size:50;unique;not null" json:"barcode"`
	Name          		string     	`gorm:"size:255;not null" json:"name"`
	Price         		float64    	`gorm:"type:decimal(18,2); not null" json:"price"`
	Quantity      		int64      	`gorm:"not null" json:"quantity"`
	Status        		string     	`gorm:"type:enum('display','sale','bkl');size:15;not null" json:"status"` 
	TagColor    		string     	`gorm:"index" json:"tag_color"`
	IsSo          		*string     `gorm:"type:enum('check','done','lost','addition');size:8" json:"is_so"`
	IsExtraProduct    	bool     	`gorm:"default:false" json:"is_extra_product"`
	UserSo        		*uint64     `json:"user_so"`
	
	CreatedAt     		time.Time   `json:"created_at"`
	UpdatedAt     		time.Time   `json:"updated_at"`

	//relations
	Store		*StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store,omitempty"`

}

func (p *Product) GetDaysSinceCreated() string {
	// Menghitung selisih waktu dari CreatedAt sampai sekarang
	duration := time.Since(p.CreatedAt)
	
	// Konversi durasi ke jam lalu bagi 24 untuk dapat jumlah hari
	days := int(duration.Hours() / 24)

	return fmt.Sprintf("%d Hari", days)
}

func (s *Product) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}
