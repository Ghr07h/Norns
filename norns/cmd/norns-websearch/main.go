package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Ghr07h/norns/norns/internal/websearch"
)

const searchTimeout = 30 * time.Second

type searchClient interface {
	Search(context.Context, string, int) ([]websearch.Result, error)
}

type clientFactory func(string) (searchClient, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, func(proxy string) (searchClient, error) {
		return websearch.New(proxy)
	}))
}

func run(args []string, stdout, stderr io.Writer, factory clientFactory) int {
	flags := flag.NewFlagSet("norns-websearch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query, proxy string
	maximum := websearch.DefaultMaxResults
	flags.StringVar(&query, "query", "", "search query")
	flags.StringVar(&proxy, "proxy", "", "HTTP, SOCKS5, or SOCKS5H proxy URL")
	flags.IntVar(&maximum, "m", websearch.DefaultMaxResults, "maximum number of results")
	flags.IntVar(&maximum, "max-results", websearch.DefaultMaxResults, "maximum number of results")
	flags.Usage = func() {}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
		return writeError(stderr, &websearch.Error{Code: "invalid_input", Message: err.Error(), Cause: err})
	}
	if flags.NArg() != 0 {
		return writeError(stderr, &websearch.Error{Code: "invalid_input", Message: "unexpected positional arguments"})
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return writeError(stderr, &websearch.Error{Code: "invalid_input", Message: "query must not be empty"})
	}
	if maximum < 1 {
		return writeError(stderr, &websearch.Error{Code: "invalid_input", Message: "max results must be a positive integer"})
	}
	client, err := factory(proxy)
	if err != nil {
		return writeError(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	results, err := client.Search(ctx, query, maximum)
	if err != nil {
		return writeError(stderr, err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(results); err != nil {
		return writeError(stderr, &websearch.Error{Code: "search_error", Message: "cannot encode search results", Cause: err})
	}
	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: norns websearch --query QUERY [-m MAX_RESULTS] [--proxy PROXY_URL]")
}

func writeError(w io.Writer, err error) int {
	code := websearch.ErrorCode(err)
	message := err.Error()
	body := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	body.Error.Code = code
	body.Error.Message = message
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(body)
	if code == "invalid_input" {
		return 2
	}
	return 1
}
