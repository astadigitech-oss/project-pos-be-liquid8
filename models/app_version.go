package models

import "time"

type AppRelease struct {
	ID uint64 `gorm:"primaryKey" json:"id"`

	Version string `gorm:"type:varchar(50);unique;not null" json:"version"`
	Notes   *string `gorm:"type:text" json:"notes"`

	WindowsInstaller string `gorm:"type:varchar(255);not null" json:"windows_installer"`
	WindowsUpdater   string `gorm:"type:varchar(255);not null" json:"windows_updater"`
	WindowsSignature string `gorm:"type:text;not null" json:"windows_signature"`

	MacInstaller string `gorm:"type:varchar(255);not null" json:"mac_installer"`
	MacUpdater   string `gorm:"type:varchar(255);not null" json:"mac_updater"`
	MacSignature string `gorm:"type:text;not null" json:"mac_signature"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *AppRelease) ToLocal(tz string) {
	loc, _ := time.LoadLocation(tz)
	s.CreatedAt = s.CreatedAt.In(loc)
	s.UpdatedAt = s.UpdatedAt.In(loc)
}