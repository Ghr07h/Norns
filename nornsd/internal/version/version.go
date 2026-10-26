package version

import "time"

const (
	APIVersion         = "v1"
	DatabaseSchema     = 1
	AgentConfigVersion = 1
	NornsProtocol      = 2
	ClaudeCodeVersion  = "2.1.199"
	SQLiteAppID        = 0x4E534456 // NSDV

	MaxInstructionBytes = 1_048_576
	MaxRequestBodyBytes = 2_097_152
	MaxPendingCount     = 16
	MaxAgentFrameBytes  = 16 * 1024 * 1024
	DefaultListLimit    = 50
	MaxListLimit        = 200
	DefaultEventLimit   = 5000
	MaxEventLimit       = 5000

	RequestTimeout         = 5 * time.Second
	ControlTimeout         = 5 * time.Second
	GracefulStopTimeout    = 30 * time.Second
	NornsCloseGrace        = time.Minute
	ResourceSampleInterval = 500 * time.Millisecond
)

var (
	ProductVersion = "dev"
	Commit         = "unknown"
	BuildTime      = "unknown"
)
