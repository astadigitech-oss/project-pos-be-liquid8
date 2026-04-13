package models

import "time"

type StoreProfile struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Timezone		string	   `json:"timezone" gorm:"size:50;not null"`
	Token           string     `json:"token" gorm:"size:25;not null"`
	StoreName		string	   `json:"store_name" gorm:"size:50;not null"`
	Phone        	string     `json:"phone" gorm:"size:15;not null"`
	Address         string     `json:"address" gorm:"type:text;not null"`
	
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *StoreProfile) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}