package models

import "time"

type Packaging struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	StoreID   *uint64    `json:"store_id" gorm:"index;"`
	Name      string    `json:"name" gorm:"type:varchar(100);not null"`
	Price     float64   `json:"price" gorm:"type:decimal(15,2);default:0"`

	//relation
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}