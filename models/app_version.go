package models

import "time"

type AppVersion struct {
	ID      uint64 `gorm:"primaryKey"`
	Version string
	Notes   string

	WindowsUpdater string
	MacUpdater     string

	WindowsInstaller string
	MacInstaller     string

	WindowsSignature string
	MacSignature     string

	CreatedAt time.Time
}