package models

import "time"

type Shift struct {
	ID      uint64 `json:"id" gorm:"primaryKey"`
	StoreID uint64 `json:"store_id" gorm:"index;not null"`
	OpenBy  uint64 `json:"open_by" gorm:"index;not null"`
	ClosedBy  *uint64 `json:"closed_by" gorm:"index;"`
	
	StartTime 	time.Time  	`json:"start_time" gorm:"type:datetime;not null"`
	EndTime   	*time.Time 	`json:"end_time,omitempty" gorm:"type:datetime"`
	Status 		string 	 	`json:"status" gorm:"type:enum('open','closed');default:'open'"`
	
	InitialCash  float64 `json:"initial_cash" gorm:"type:decimal(15,2);default:0"` //saldo awal mulai shift
	TotalCash float64 `json:"total_cash" gorm:"type:decimal(15,2);default:0"` //total penjualan metode cash
	TotalTransfer float64 `json:"total_transfer" gorm:"type:decimal(15,2);default:0"` //total penjualan metode transfer
	TotalQris	 float64 `json:"total_qris" gorm:"type:decimal(15,2);default:0"`		//total penjualan metode qris
	TotalTax	 float64 `json:"total_tax" gorm:"type:decimal(15,2);default:0"`		//total pajak dari semua transaksi di shift ini

	Subtotal   float64 `json:"subtotal" gorm:"type:decimal(15,2);default:0"` //total penjualan dari semua transaksi sebelum pajak
	ExpectedAmount float64 `json:"expected_amount" gorm:"type:decimal(15,2);default:0"` //(total semua penjualan + total_pajak) + initalcash
	ActualCash   float64 `json:"actual_cash" gorm:"type:decimal(15,2);default:0"`	//total kas sebenarnya di kasir
	Difference   float64 `json:"difference" gorm:"type:decimal(15,2);default:0"`	// selisih
	Note        *string  `json:"note" gorm:"type:text"`

	//relation
	Store   *StoreProfile `gorm:"foreignKey:StoreID;references:ID" json:"store,omitempty"`
	UserOpen    *User   `gorm:"foreignKey:OpenBy;references:ID" json:"user_open,omitempty"`
	UserClosed    *User   `gorm:"foreignKey:ClosedBy;references:ID" json:"user_closed,omitempty"`
	Transactions []Transaction `json:"transactions,omitempty" gorm:"foreignKey:ShiftID;references:ID"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Shift) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
	s.StartTime = s.StartTime.In(loc)
	if s.EndTime != nil {
		*s.EndTime = s.EndTime.In(loc)
	}
}