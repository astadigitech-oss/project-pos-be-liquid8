package models

import "time"

type Member struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	StoreID			*uint64	   `json:"store_id" gorm:"index"`
	Code            string     `json:"code" gorm:"size:15;not null"`
	Name        	string     `json:"name" gorm:"size:255;not null"`
	Phone        	string     `json:"phone" gorm:"size:15;not null"`
	
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// relations
	Store *StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store,omitempty"`
}

