package controlplane

import "time"

type lspProcessConfig struct {
	Executable string
	Args       []string
	Timeout    time.Duration
}

type lspProcessHandle struct {
	Language string
	Started  time.Time
}
