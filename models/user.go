package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	StoreID			*uint64	   `json:"store_id" gorm:"index"`
	Name            string     `json:"name" gorm:"size:255;not null"`
	Username        string     `json:"username" gorm:"size:255;unique;not null"`
	Email           string     `json:"email" gorm:"size:255;unique;not null"`
	Password        string     `json:"-" gorm:"size:255;not null"`
	RememberToken   string     `json:"-" gorm:"size:100"`
	Role            string     `json:"role" gorm:"type:enum('superadmin','admin','kasir');not null"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DeletedAt 		gorm.DeletedAt `gorm:"index" json:"-"`

	// relations
	Store *StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store_,omitempty"`
}

func (s *User) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}

