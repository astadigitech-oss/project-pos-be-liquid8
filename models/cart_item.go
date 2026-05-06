package models

import "time"

type CartItem struct {
	ID        uint64 `json:"id" gorm:"primaryKey"`
	CartID   uint64 `json:"cart_id" gorm:"index;not null"`
	
	ProductID *uint64 `json:"product_id" gorm:"index;"`
	PackagingID *uint64 `json:"packaging_id" gorm:"index;"`

	ProductName string  `json:"product_name"`
	Quantity    uint64   `json:"quantity" gorm:"default:0"`
	Price       float64 `json:"price" gorm:"type:decimal(15,2);default:0"`
	DiscountPrice float64 `json:"discount_price" gorm:"type:decimal(15,2);default:0"`
	Subtotal    float64 `json:"subtotal" gorm:"type:decimal(15,2);default:0"`
	Type		string  `json:"type" gorm:"type:enum('product','packaging');default:'product'"`

	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`

	//relation
	Product   *Product `gorm:"foreignKey:ProductID;references:ID" json:"product,omitempty"`
	Packaging   *Packaging `gorm:"foreignKey:PackagingID;references:ID" json:"packaging,omitempty"`
}

func (s *CartItem) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}