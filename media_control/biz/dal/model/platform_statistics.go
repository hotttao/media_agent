package model

import "time"

type PlatformStatistic struct {
	MetricKey      string
	PeriodStart    time.Time
	Dimensions     string `gorm:"type:jsonb"`
	DimensionsHash string
	Value          string `gorm:"type:numeric"`
	RefreshedAt    time.Time
}

func (PlatformStatistic) TableName() string { return "platform_statistics" }
