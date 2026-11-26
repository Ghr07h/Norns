package root

import (
	"path/filepath"
	"strings"
)

// SubjectDataPath maps an immutable UUID_timestamp workspace name to its
// control data. Agent working directories never contain daemon credentials.
func SubjectDataPath(workspace string) string {
	id := strings.SplitN(filepath.Base(workspace), "_", 2)[0]
	return filepath.Join(filepath.Dir(workspace), ".nornsd", "subjects", id)
}
func RuntimeDataPath(workspace, runtimeID string) string {
	return filepath.Join(SubjectDataPath(workspace), "runtimes", runtimeID)
}
func (o *Owner) SubjectData(subjectID string) string {
	return filepath.Join(o.Control, "subjects", subjectID)
}
