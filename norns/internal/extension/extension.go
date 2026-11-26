package extension

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/version"
)

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func IsValidName(name string) bool { return validName.MatchString(name) }

func Find(name string) (string, error) {
	if !IsValidName(name) {
		return "", domain.NewError("INVALID_ARGUMENT", "invalid command name")
	}
	directory, err := executableDirectory()
	if err != nil {
		return "", err
	}
	return findInDirectory(directory, name)
}

func findInDirectory(directory, name string) (string, error) {
	if !IsValidName(name) {
		return "", domain.NewError("INVALID_ARGUMENT", "invalid command name")
	}
	base := "norns-" + name
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	candidate := filepath.Join(directory, base)
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
		return candidate, nil
	}
	e := domain.NewError("EXTENSION_NOT_FOUND", "Norns extension not found: "+name)
	e.Details["executable"] = base
	return "", e
}

func executableDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot locate Norns executable", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot resolve Norns executable", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot resolve Norns executable path", err)
	}
	return filepath.Dir(executable), nil
}

func Run(path string, args []string, stdin io.Reader, stdout, stderr io.Writer, workspace string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Dir = workspace
	executable, _ := os.Executable()
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"NORNS_EXECUTABLE="+executable,
		"NORNS_WORKSPACE="+workspace,
		"NORNS_PROTOCOL_VERSION="+strings.TrimSpace(itoa(version.ProtocolVersion)),
		"NORNS_SCHEMA_VERSION="+strings.TrimSpace(itoa(version.SchemaVersion)),
	)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return &ExitError{Code: exit.ExitCode()}
		}
		return domain.Wrap("IO_ERROR", "cannot execute Norns extension", err)
	}
	return nil
}

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return "extension exited unsuccessfully" }

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	b := [20]byte{}
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
