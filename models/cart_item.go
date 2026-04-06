package models

import "time"

type CartItem struct {
	ID        uint64 `json:"id" gorm:"primaryKey"`
	StoreID   uint64 `json:"store_id" gorm:"index;not null"`
	UserID    uint64 `json:"user_id" gorm:"index; not null"`
	ProductID uint64 `json:"product_id" gorm:"index;not null"`

	KeepCode    *string  `json:"keep_code" gorm:"size:25;index"`
	ProductName string  `json:"product_name"`
	Quantity    int64   `json:"quantity" gorm:"default:0"`
	Price       float64 `json:"price" gorm:"type:decimal(15,2);default:0"`
	DiscountPrice float64 `json:"discount_price" gorm:"type:decimal(15,2);default:0"`
	Subtotal    float64 `json:"subtotal" gorm:"type:decimal(15,2);default:0"`

	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`

	//relation
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
	User   *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Product   *Product `gorm:"foreignKey:ProductID;references:ID" json:"product,omitempty"`
}