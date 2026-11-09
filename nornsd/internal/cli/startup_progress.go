package cli

import (
	"fmt"
	"io"
	"time"
)

const serverStartupProgressInterval = 5 * time.Second

func runServerStartupPhase(out io.Writer, phase string, interval time.Duration, work func() error) error {
	started := time.Now()
	_, _ = fmt.Fprintf(out, "nornsd: %s...\n", phase)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = fmt.Fprintf(out, "nornsd: still %s (elapsed %s; HTTP API is not ready yet)\n", phase, time.Since(started).Round(time.Second))
			}
		}
	}()
	err := func() error {
		defer func() {
			close(stop)
			<-done
		}()
		return work()
	}()
	if err != nil {
		_, _ = fmt.Fprintf(out, "nornsd: %s failed after %s\n", phase, time.Since(started).Round(time.Second))
	} else {
		_, _ = fmt.Fprintf(out, "nornsd: %s completed in %s\n", phase, time.Since(started).Round(time.Second))
	}
	return err
}
