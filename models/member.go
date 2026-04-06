package models

import "time"

type Member struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	StoreID			uint64	   `json:"store_id" gorm:"index"`
	Code            string     `json:"code" gorm:"size:15;not null"`
	Name        	string     `json:"name" gorm:"size:255;not null"`
	Email           string     `json:"email" gorm:"size:255;unique;not null"`
	Phone        	string     `json:"phone" gorm:"size:15;not null"`
	NoKtp   		string     `json:"no_ktp" gorm:"unique;size:16;not null"`
	
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// relations
	Store *StoreProfile `gorm:"foreignKey:StoreID;references:ID;constraint:OnDelete:CASCADE" json:"store,omitempty"`
}

