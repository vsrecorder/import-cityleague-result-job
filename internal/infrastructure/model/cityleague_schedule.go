package model

import (
	"time"
)

type CityleagueSchedule struct {
	ID       string
	Title    string
	FromDate time.Time
	ToDate   time.Time
}
