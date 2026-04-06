package models

import "time"

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

	// relations
	Store *StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store_,omitempty"`
}

