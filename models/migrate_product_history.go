package models

import "time"

type MigrateProductHistory struct {
	ID      		uint64 `json:"id" gorm:"primaryKey"`
	StoreID 		uint64 `json:"store_id" gorm:"index;not null"`
	
	User           	string `json:"user" gorm:"not null"`
	TotalProduct   	int     `json:"total_product" gorm:"default:0"`
	TotalQuantity  	int64   `json:"total_quantity" gorm:"default:0"`
	TotalPrice     	float64 `json:"total_price" gorm:"type:decimal(15,2);default:0"`
	TypeMigration string    `json:"type_migration" gorm:"type:enum('IN','OUT')"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	//relation
	Products []Product `json:"products,omitempty" gorm:"foreignKey:MigrateID;references:ID"`
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
}