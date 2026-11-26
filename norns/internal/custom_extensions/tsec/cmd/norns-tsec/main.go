package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Ghr07h/norns/norns/internal/custom_extensions/tsec"
)

type submitClient interface {
	Submit(context.Context, tsec.SubmitRequest) (json.RawMessage, error)
}

type clientFactory func() (submitClient, error)

func configuredClient() (submitClient, error) {
	config, err := tsec.LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return tsec.New(config)
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, configuredClient)) }

func run(args []string, stdout, stderr io.Writer, factory clientFactory) int {
	flags := flag.NewFlagSet("norns-tsec", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var request tsec.SubmitRequest
	flags.StringVar(&request.UniqueCode, "unique-code", "", "challenge unique code (required)")
	flags.StringVar(&request.Flag, "flag", "", "flag to submit, 1 to 4096 Unicode characters (required)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
		return writeError(stderr, &tsec.Error{Code: "invalid_input", Message: err.Error()})
	}
	if flags.NArg() != 0 {
		return writeError(stderr, &tsec.Error{Code: "invalid_input", Message: "unexpected positional arguments"})
	}
	if err := request.Validate(); err != nil {
		return writeError(stderr, err)
	}
	client, err := factory()
	if err != nil {
		return writeError(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), tsec.RequestTimeout)
	defer cancel()
	result, err := client.Submit(ctx, request)
	if err != nil {
		return writeError(stderr, err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return writeError(stderr, &tsec.Error{Code: "output_error", Message: "cannot write TSecBench result; the platform has already processed the submission"})
	}
	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: norns tsec --unique-code CODE --flag FLAG

Submit one flag to TSecBench. Both options are required.
Configuration: BENCHMARK_BASE_URL (HTTP(S) service base URL) and BENCHMARK_TOKEN.
The plugin GETs /openapi/v1/challenges and submits only if the challenge container is available.
It then POSTs to /openapi/v1/challenges/submit; both requests use BENCHMARK_TOKEN authentication.
Flag values are sent unchanged, with a length of 1 to 4096 Unicode characters.
The status check and submission share a 60-second timeout; neither is retried automatically.
Success: native platform JSON on stdout, including correct=false for an incorrect flag.
An HTTP 409 duplicate submission returns correct=true and duplicate=true on stdout, with exit code 0.
Duplicate results omit scores and progress that the platform did not return.
Errors: structured JSON on stderr, preserving business error codes and HTTP status.
An unavailable container returns container_not_ready with its status; no flag is submitted.
Exit codes: 0 completed request/help, 2 invalid CLI input, 1 configuration/API/transport/output error.`)
}

func writeError(w io.Writer, err error) int {
	var detail *tsec.Error
	if !errors.As(err, &detail) {
		detail = &tsec.Error{Code: "request_error", Message: "TSecBench flag submission failed"}
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(struct {
		Error *tsec.Error `json:"error"`
	}{detail})
	if detail.Code == "invalid_input" && detail.Status == 0 {
		return 2
	}
	return 1
}
