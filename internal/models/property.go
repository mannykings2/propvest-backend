package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type Property struct {
    ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    PropCode        string         `gorm:"uniqueIndex;not null"` // e.g. PROP-001
    Name            string         `gorm:"not null"`
    Location        string         `gorm:"not null"`
    State           string         `gorm:"not null"`
    Type            string         `gorm:"not null"` // Residential|Apartment|Land|Commercial
    IncomeType      string         `gorm:"not null"` // rental|resale
    SPVName         string         `gorm:"not null"`
    SPVCACNo        string
    TotalSlots      int            `gorm:"not null"`
    SlotPrice       int64          `gorm:"not null"` // in kobo
    TotalValue      int64          `gorm:"not null"`
    PurchasePrice   int64          `gorm:"not null"`
    YieldPct        float64
    AnnualRent      *int64
    MonthlyRent     *int64
    FundedPct       int            `gorm:"default:0"`
    SlotsSold       int            `gorm:"default:0"`
    HoldYears       int            `gorm:"default:3"`
    Status          string         `gorm:"default:'draft'"` // draft|funding|live|sold|closed
    DeveloperID     *uuid.UUID     `gorm:"type:uuid"`
    Description     string
    Tag             string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt `gorm:"index"`
}