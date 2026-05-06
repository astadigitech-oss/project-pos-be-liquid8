package models

import "time"

type Cart struct {
	ID uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	MemberID  *uint `json:"member_id" gorm:"index;"`
	StoreID uint64 `json:"store_id" gorm:"index;not null"`
	UserID  uint64 `json:"user_id" gorm:"index;not null"`

	KeepCode    *string  `json:"keep_code" gorm:"size:25;index"`
	Subtotal   float64 `json:"subtotal" gorm:"type:decimal(15,2);default:0"`
	Discount float64 `json:"discount" gorm:"type:decimal(15,2);default:0"`
	GrandTotal float64 `json:"grand_total" gorm:"type:decimal(15,2);default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	//relation
	Items []CartItem `json:"items,omitempty" gorm:"foreignKey:CartID;references:ID"`
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
	User   *User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Member   *Member `gorm:"foreignKey:MemberID;references:ID" json:"member,omitempty"`
}

func (s *Cart) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}