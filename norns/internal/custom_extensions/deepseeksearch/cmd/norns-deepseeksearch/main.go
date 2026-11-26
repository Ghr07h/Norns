package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Ghr07h/norns/norns/internal/custom_extensions/deepseeksearch"
)

type searchClient interface {
	Search(context.Context, deepseeksearch.SearchRequest) (*deepseeksearch.SearchResponse, error)
	Fetch(context.Context, string) (*deepseeksearch.FetchResponse, error)
}

type clientFactory func() (searchClient, error)

func configuredClient() (searchClient, error) {
	config, err := deepseeksearch.LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return deepseeksearch.New(config)
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, configuredClient)) }

func run(args []string, stdout, stderr io.Writer, factory clientFactory) int {
	command := "search"
	if len(args) > 0 && (args[0] == "search" || args[0] == "fetch") {
		command, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("norns-deepseeksearch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var request deepseeksearch.SearchRequest
	var target string
	if command == "search" {
		flags.StringVar(&request.Query, "query", "", "search query (1 to 128 Unicode characters)")
		flags.IntVar(&request.Count, "count", deepseeksearch.DefaultCount, "number of results (1 to 50)")
		flags.IntVar(&request.Count, "m", deepseeksearch.DefaultCount, "alias for --count")
		flags.IntVar(&request.Count, "max-results", deepseeksearch.DefaultCount, "alias for --count")
		flags.StringVar(&request.OriQuestion, "ori-question", "", "original user question (1 to 128 Unicode characters)")
		flags.StringVar(&request.Localization, "localization", "", "IP address for localized results")
		flags.BoolVar(&request.NeedContent, "need-content", false, "include page content")
	} else {
		flags.StringVar(&target, "url", "", "HTTP(S) URL to fetch")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
		return writeError(stderr, &deepseeksearch.Error{Code: "invalid_input", Message: err.Error()})
	}
	if flags.NArg() != 0 {
		return writeError(stderr, &deepseeksearch.Error{Code: "invalid_input", Message: "unexpected positional arguments"})
	}
	var err error
	if command == "search" {
		err = request.Validate()
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "ori-question" && request.OriQuestion == "" {
				err = &deepseeksearch.Error{Code: "invalid_input", Message: "ori-question must not be empty when provided"}
			}
		})
	} else {
		err = deepseeksearch.ValidateFetchURL(target)
	}
	if err != nil {
		return writeError(stderr, err)
	}
	client, err := factory()
	if err != nil {
		return writeError(stderr, err)
	}
	var result any
	if command == "search" {
		result, err = client.Search(context.Background(), request)
	} else {
		result, err = client.Fetch(context.Background(), target)
	}
	if err != nil {
		return writeError(stderr, err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return writeError(stderr, &deepseeksearch.Error{Code: "output_error", Message: "cannot write DeepSeek results"})
	}
	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: norns deepseeksearch [search] --query QUERY [--count N] [--need-content] [--ori-question QUESTION] [--localization IP]
       norns deepseeksearch fetch --url URL

Search options:
  --query QUERY           Required; 1 to 128 Unicode characters. Search operators are passed through.
  --count N               Number of results, 1 to 50 (default 10); aliases: -m, --max-results.
  --need-content          Include page content in search results (default false).
  --ori-question QUESTION Original user question, 1 to 128 Unicode characters.
  --localization IP       IP address for localized results.

Configuration: NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG must name a YAML or JSON file.
Fields: api_key (required), base_url (default https://api.deepseek.com), timeout_seconds (default 30).
Success: native DeepSeek JSON; errors: structured JSON on stderr. Exit codes: 0 success, 2 invalid input, 1 other errors.`)
}

func writeError(w io.Writer, err error) int {
	var detail *deepseeksearch.Error
	if !errors.As(err, &detail) {
		detail = &deepseeksearch.Error{Code: "request_error", Message: "DeepSeek request failed"}
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(struct {
		Error *deepseeksearch.Error `json:"error"`
	}{detail})
	if detail.Code == "invalid_input" {
		return 2
	}
	return 1
}
