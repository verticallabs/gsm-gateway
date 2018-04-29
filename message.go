package main

import (
	"time"
)

type Message struct {
	ID        string    `gorm:"primary_key,size:32" json:"id"`
	Number    string    `gorm:"size:32" json:"number"`
	Body      string    `gorm:"size:160" json:"body"`
	Incoming  bool      `gorm:"index" json:"-"`
	Handled   bool      `gorm:"index" json:"-"`
	Time      time.Time `json:"time"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

func readTimeAsUTC(localTime time.Time, location *time.Location) time.Time {
	str := localTime.Format("2006/01/02,15:04:05")
	t, _ := time.ParseInLocation("2006/01/02,15:04:05", str, location)
	return t.UTC()
}
