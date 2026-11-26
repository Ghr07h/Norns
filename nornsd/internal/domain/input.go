package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// InputUUID identifies a committed dispatch group across write, replay and
// result settlement. It is stable and belongs exclusively to this Runtime.
func InputUUID(runtimeID string, group int64) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("nornsd:%s:%d", runtimeID, group))).String()
}
