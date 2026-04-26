package models

import (
	"time"

	"gorm.io/gorm"
)

type Member struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	StoreID			*uint64	   `json:"store_id" gorm:"index"`
	Code            string     `json:"code" gorm:"size:15;not null"`
	Name        	string     `json:"name" gorm:"size:255;not null"`
	Phone        	string     `json:"phone" gorm:"size:15;not null"`
	
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DeletedAt 			gorm.DeletedAt `gorm:"index" json:"-"`

	// relations
	Store *StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store,omitempty"`
}

func (s *Member) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}