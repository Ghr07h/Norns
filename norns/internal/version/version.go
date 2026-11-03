package version

import versions "github.com/Ghr07h/norns"

const (
	SQLiteAppID = 0x4E4F524E
)

var (
	manifest        = versions.Read()
	ProtocolVersion = manifest.Norns.SupportProtocol
	SchemaVersion   = manifest.Norns.DatabaseSchema
	ProductVersion  = manifest.Norns.Version
	Commit          = "unknown"
	BuildTime       = "unknown"
)
