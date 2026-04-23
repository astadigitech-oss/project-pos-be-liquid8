package models

import "time"

type UserToken struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	UserID     uint      `gorm:"not null"`
	Token      string    `gorm:"type:text;not null"`
	UserAgent  string    `gorm:"size:255"`
	LastUsedAt time.Time `gorm:"not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"` // auto input
}

func (s *UserToken) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.LastUsedAt = s.LastUsedAt.In(loc)
	s.CreatedAt = s.CreatedAt.In(loc)
}