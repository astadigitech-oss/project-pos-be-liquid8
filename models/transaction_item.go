package models

import "time"

type TransactionItem struct {
	ID            	uint64 `json:"id" gorm:"primaryKey"`
	StoreID 		uint64 `json:"store_id" gorm:"index;not null"`
	TransactionID 	uint64 `json:"transaction_id" gorm:"index;not null"`
	ProductID *uint64 `json:"product_id" gorm:"index;"`
	PackagingID *uint64 `json:"packaging_id" gorm:"index;"`

	ProductName  	string  `json:"product_name"`
	Quantity     	uint64   	`json:"quantity" gorm:"default:0"`
	Price        	float64 `json:"price" gorm:"type:decimal(18,2);default:0"`
	DiscountPrice 	float64 `json:"discount_price" gorm:"type:decimal(18,2);default:0"`
	Subtotal     	float64 `json:"subtotal" gorm:"type:decimal(18,2);default:0"`
	Type			string  `json:"type" gorm:"type:enum('product','packaging');default:'product'"`

	//relation
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
	Product   *Product `gorm:"foreignKey:ProductID;references:ID" json:"product,omitempty"`
	Packaging   *Packaging `gorm:"foreignKey:PackagingID;references:ID" json:"packaging,omitempty"`

	CreatedAt 		time.Time `json:"created_at"`
	UpdatedAt 		time.Time `json:"updated_at"`
}

func (s *TransactionItem) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}