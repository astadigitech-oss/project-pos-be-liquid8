package models

import "time"

type Ppn struct {
	ID                     	uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Ppn						float64		`gorm:"not null;type:decimal(15,2)" json:"ppn"`
	IsTaxDefault			bool		`gorm:"default:false;comment:Penanda PPN yang aktif" json:"is_tax_default"`
	CreatedAt              time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (s *Ppn) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}
