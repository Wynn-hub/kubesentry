package dbmodel

import (
	"time"

	"gorm.io/gorm"
)

// Base is embedded by every GORM model in the kubesentry schema. ID is a
// snowflake value assigned in BeforeCreate — never DB-generated — so the
// caller knows the row's ID immediately after Create() without a second
// round trip (spec D4).
type Base struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == 0 {
		b.ID = NewID()
	}
	return nil
}
