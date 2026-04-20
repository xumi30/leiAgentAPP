package crontabthread

import "time"

type ScheduledTask struct {
	ID            string
	UserID        string
	Title         string
	ActionType    string
	ActionPayload string
	ScheduleType  string
	RunAt         time.Time
	CronExpr      string
	RRule         string
	Timezone      string
	Status        string
	Executing     bool
	Version       int
	Completed     bool
	RunCount      int
	LastRunAt     time.Time
	NextRunAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

