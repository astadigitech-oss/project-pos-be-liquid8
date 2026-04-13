package models

import "time"

type UserToken struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	UserID     uint      `gorm:"not null"`
	Token      string    `gorm:"type:text;not null;unique"`
	UserAgent  string    `gorm:"size:255"`
	LastUsedAt time.Time `gorm:"not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"` // auto input
}