package qpool

import "time"

type Config struct {
	SchedulingTimeout time.Duration
}

func NewConfig() *Config {
	return &Config{
		SchedulingTimeout: 10 * time.Second,
	}
}
