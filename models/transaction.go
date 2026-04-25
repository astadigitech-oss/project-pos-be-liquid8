package models

import "time"

type Transaction struct {
	ID      uint64 `json:"id" gorm:"primaryKey"`
	StoreID uint64 `json:"store_id" gorm:"index;not null"`
	UserID  uint64 `json:"user_id" gorm:"index;not null"`
	ShiftID uint64 `json:"shift_id" gorm:"index;not null"`
	MemberID uint64  `json:"member_id" gorm:"index"`

	Invoice string `json:"invoice" gorm:"unique;size:25;not null"`
	TotalItem     int `json:"total_item" gorm:"default:0;"`
	TotalQuantity int `json:"total_quantity" gorm:"default:0"`
	MemberPoint   int `json:"member_point" gorm:"default:0"`
	Subtotal     float64 `json:"subtotal" gorm:"type:decimal(18,2);default:0"`
	Tax          float64 `json:"tax" gorm:"type:decimal(18,2);default:0"`
	TaxPrice	 float64 `json:"tax_price" gorm:"type:decimal(18,2);default:0"`
	Discount     float64 `json:"discount" gorm:"type:decimal(18,2);default:0"`
	TotalAmount  float64 `json:"total_amount" gorm:"type:decimal(18,2);default:0"`
	PaidAmount   float64 `json:"paid_amount" gorm:"type:decimal(18,2);default:0"`
	ChangeAmount float64 `json:"change_amount" gorm:"type:decimal(18,2);default:0"`
	PaymentMethod string `json:"payment_method" gorm:"type:enum('cash','transfer','qris')"`
	Status        string `json:"status" gorm:"type:enum('pending','cancelled','done');default:'pending'"`

	//relation
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
	User   *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Member   *Member `gorm:"foreignKey:MemberID;references:ID" json:"member,omitempty"`
	Items []TransactionItem `json:"items,omitempty" gorm:"foreignKey:TransactionID;references:ID"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Transaction) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}