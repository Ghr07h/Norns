package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Ghr07h/norns/norns/internal/app"
	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/extension"
	"github.com/Ghr07h/norns/norns/internal/i18n"
	"github.com/Ghr07h/norns/norns/internal/output"
	"github.com/Ghr07h/norns/norns/internal/version"
)

var coreCommands = map[string]bool{"workspace": true, "task": true, "cognition": true, "exploration": true, "view": true, "version": true, "extension": true, "help": true, "completion": true}

const structuredViewInlineLimit = 6 * 1024

type publishedView struct {
	Path      string `json:"path"`
	Format    string `json:"format"`
	SizeBytes int    `json:"size_bytes"`
	Note      string `json:"note"`
}

func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	rootPath, err := os.Getwd()
	if err != nil {
		return output.WriteError(stderr, domain.Wrap("IO_ERROR", "cannot resolve current directory", err))
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && !coreCommands[args[0]] {
		path, findErr := extension.Find(args[0])
		if findErr != nil {
			return output.WriteError(stderr, findErr)
		}
		err := extension.Run(path, args[1:], stdin, stdout, stderr, rootPath)
		if err == nil {
			return 0
		}
		var exit *extension.ExitError
		if errors.As(err, &exit) {
			return exit.Code
		}
		return output.WriteError(stderr, err)
	}
	service, err := app.New(rootPath)
	if err != nil {
		return output.WriteError(stderr, err)
	}
	root := newRoot(service, stdout, stderr)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		return output.WriteError(stderr, normalizeCLIError(err))
	}
	return 0
}

func newRoot(service *app.Service, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{Use: "norns", Short: "Norns protocol Workspace CLI", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(workspaceCommand(service, stdout), taskCommand(service, stdout), cognitionCommand(service, stdout), explorationCommand(service, stdout), viewCommand(service, stdout), versionCommand(stdout), extensionCommand(stdout))
	root.CompletionOptions.DisableDefaultCmd = true
	return root
}

func extensionCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "extension", Short: "Inspect external command extensions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List discoverable extension names and executable paths", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		found, err := extension.List()
		if err != nil {
			return err
		}
		available := make([]extension.Info, 0, len(found))
		for _, item := range found {
			if !coreCommands[item.Name] {
				available = append(available, item)
			}
		}
		var warnings []output.Warning
		if err := extension.Enrich(available); err != nil {
			warnings = append(warnings, output.Warning{Code: "EXTENSION_METADATA_IGNORED", Message: "Extension metadata could not be read or validated; displaying names and paths only", Details: map[string]any{}})
		}
		return output.WriteSuccess(out, available, warnings, "Extensions listed")
	}})
	return cmd
}

func workspaceCommand(s *app.Service, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "workspace"}
	cmd.AddCommand(
		&cobra.Command{Use: "check", Short: "Validate the current Workspace and summarize its Tasks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			r, e := s.CheckWorkspace(cmd.Context())
			return emit(out, r, e)
		}},
		&cobra.Command{Use: "init", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { r, e := s.Init(cmd.Context()); return emit(out, r, e) }},
		&cobra.Command{Use: "free", Short: "Cancel unfinished work and release Workspace Runtime occupancy", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			r, e := s.FreeWorkspace(cmd.Context())
			return emit(out, r, e)
		}},
		&cobra.Command{Use: "reset", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { r, e := s.Reset(cmd.Context()); return emit(out, r, e) }},
	)
	return cmd
}

func taskCommand(s *app.Service, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "task"}
	var status string
	list := &cobra.Command{Use: "list", Short: "List Tasks without generating a View", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.ListTasks(cmd.Context(), status)
		return emit(out, r, e)
	}}
	list.Flags().StringVar(&status, "status", "", "filter by Task status: open or closed (default: all)")
	var purpose, info string
	var concurrency int
	var infoSet bool
	open := &cobra.Command{Use: "open", Args: cobra.NoArgs, PreRun: func(cmd *cobra.Command, _ []string) { infoSet = cmd.Flags().Changed("info") }, RunE: func(cmd *cobra.Command, _ []string) error {
		var p *string
		if infoSet {
			p = &info
		}
		r, e := s.OpenTask(cmd.Context(), app.OpenTaskRequest{Purpose: purpose, Concurrency: concurrency, Info: p})
		return emit(out, r, e)
	}}
	open.Flags().StringVar(&purpose, "purpose", "", "Task purpose")
	open.Flags().IntVar(&concurrency, "concurrency", 0, "maximum Exploration concurrency")
	open.Flags().StringVar(&info, "info", "", "Subject information")
	_ = open.MarkFlagRequired("purpose")
	_ = open.MarkFlagRequired("concurrency")
	var outcome, reason string
	closeCmd := &cobra.Command{Use: "close", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.CloseTask(cmd.Context(), outcome, reason)
		return emit(out, r, e)
	}}
	closeCmd.Flags().StringVar(&outcome, "outcome", "", "Task outcome")
	closeCmd.Flags().StringVar(&reason, "outcome-reason", "", "Task outcome reason")
	_ = closeCmd.MarkFlagRequired("outcome")
	_ = closeCmd.MarkFlagRequired("outcome-reason")
	var taskID string
	reopen := &cobra.Command{Use: "reopen", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var p *string
		if cmd.Flags().Changed("task-id") {
			p = &taskID
		}
		r, e := s.ReopenTask(cmd.Context(), p)
		return emit(out, r, e)
	}}
	reopen.Flags().StringVar(&taskID, "task-id", "", "Task ID")
	cmd.AddCommand(list, open, closeCmd, reopen)
	return cmd
}

func cognitionCommand(s *app.Service, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "cognition"}
	var title, content, origin, explorationID, meta string
	var relevant bool
	add := &cobra.Command{Use: "add", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var exp *string
		if cmd.Flags().Changed("exploration") {
			exp = &explorationID
		}
		var metadata *string
		if cmd.Flags().Changed("meta") {
			metadata = &meta
		}
		r, e := s.AddCognition(cmd.Context(), app.AddCognitionRequest{Title: title, Content: content, Origin: origin, ExplorationID: exp, Relevant: relevant, Meta: metadata})
		return emit(out, r, e)
	}}
	add.Flags().StringVar(&title, "cognition-title", "", "Cognition title")
	add.Flags().StringVar(&content, "cognition-content", "", "Cognition content")
	add.Flags().StringVar(&meta, "meta", "", "domain metadata as a JSON object with string values")
	add.Flags().StringVar(&origin, "origin", "", "Cognition origin")
	add.Flags().StringVar(&explorationID, "exploration", "", "origin Exploration ID")
	add.Flags().BoolVar(&relevant, "task-purpose-relevant", false, "mark directly relevant to Task purpose")
	for _, f := range []string{"cognition-title", "cognition-content", "origin"} {
		_ = add.MarkFlagRequired(f)
	}
	var cognitionID string
	get := &cobra.Command{Use: "get", Short: "Get a Cognition by ID", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.GetCognition(cmd.Context(), cognitionID)
		return emit(out, r, e)
	}}
	get.Flags().StringVar(&cognitionID, "cognition", "", "Cognition ID")
	_ = get.MarkFlagRequired("cognition")
	cmd.AddCommand(add, get)
	return cmd
}

func explorationCommand(s *app.Service, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "exploration"}
	var title, content string
	var sources []string
	add := &cobra.Command{Use: "add", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.AddExploration(cmd.Context(), app.AddExplorationRequest{Title: title, Content: content, Sources: sources})
		return emit(out, r, e)
	}}
	add.Flags().StringVar(&title, "exploration-title", "", "Exploration title")
	add.Flags().StringVar(&content, "exploration-content", "", "Exploration content")
	add.Flags().StringArrayVar(&sources, "source-cognitions", nil, "source Cognition ID (repeatable)")
	_ = add.MarkFlagRequired("exploration-title")
	_ = add.MarkFlagRequired("exploration-content")
	var id string
	retry := &cobra.Command{Use: "retry", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.RetryExploration(cmd.Context(), id)
		return emit(out, r, e)
	}}
	retry.Flags().StringVar(&id, "exploration", "", "finished Exploration ID")
	_ = retry.MarkFlagRequired("exploration")
	var assignID, explorer, outputRef string
	assign := &cobra.Command{Use: "assign", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.AssignExploration(cmd.Context(), assignID, explorer, outputRef)
		return emit(out, r, e)
	}}
	assign.Flags().StringVar(&assignID, "exploration", "", "todo Exploration ID")
	assign.Flags().StringVar(&explorer, "explorer", "", "Explorer ID")
	assign.Flags().StringVar(&outputRef, "output", "", "opaque output reference")
	for _, f := range []string{"exploration", "explorer", "output"} {
		_ = assign.MarkFlagRequired(f)
	}
	var finishID, outcome, reason string
	finish := &cobra.Command{Use: "finish", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.FinishExploration(cmd.Context(), finishID, outcome, reason)
		return emit(out, r, e)
	}}
	finish.Flags().StringVar(&finishID, "exploration", "", "Exploration ID")
	finish.Flags().StringVar(&outcome, "outcome", "", "Exploration outcome")
	finish.Flags().StringVar(&reason, "outcome-reason", "", "Exploration outcome reason")
	for _, f := range []string{"exploration", "outcome", "outcome-reason"} {
		_ = finish.MarkFlagRequired(f)
	}
	todo := &cobra.Command{Use: "todo", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		v, e := s.ListExplorations(cmd.Context(), "todo")
		if e != nil {
			return e
		}
		return output.WriteSuccess(out, v, nil, "todo Explorations")
	}}
	running := &cobra.Command{Use: "running", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		v, e := s.ListExplorations(cmd.Context(), "running")
		if e != nil {
			return e
		}
		return output.WriteSuccess(out, v, nil, "running Explorations")
	}}
	var getID string
	get := &cobra.Command{Use: "get", Short: "Get an Exploration by ID", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.GetExploration(cmd.Context(), getID)
		return emit(out, r, e)
	}}
	get.Flags().StringVar(&getID, "exploration", "", "Exploration ID")
	_ = get.MarkFlagRequired("exploration")
	cmd.AddCommand(add, get, retry, assign, finish, todo, running)
	return cmd
}

func viewCommand(s *app.Service, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "view"}
	var statusTask string
	var statusJSON, statusMarkdown bool
	var statusReference bool
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var id *string
		if cmd.Flags().Changed("task-id") {
			id = &statusTask
		}
		format := structuredViewFormat(statusJSON, statusMarkdown)
		b, path, e := s.StatusView(cmd.Context(), id, format)
		if e != nil {
			return e
		}
		return emitStructuredView(out, b, path, format, statusReference)
	}}
	status.Flags().StringVar(&statusTask, "task-id", "", "Task context")
	status.Flags().BoolVar(&statusJSON, "json", false, "emit JSON")
	status.Flags().BoolVar(&statusMarkdown, "md", false, "emit Markdown")
	status.MarkFlagsMutuallyExclusive("json", "md")
	status.Flags().BoolVar(&statusReference, "output-reference", false, "emit the published View reference")
	_ = status.Flags().MarkHidden("output-reference")
	var traceID string
	var traceJSON, traceMarkdown bool
	var traceReference bool
	trace := &cobra.Command{Use: "trace", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		format := structuredViewFormat(traceJSON, traceMarkdown)
		b, path, e := s.TraceView(cmd.Context(), traceID, format)
		if e != nil {
			return e
		}
		return emitStructuredView(out, b, path, format, traceReference)
	}}
	trace.Flags().StringVar(&traceID, "exploration", "", "Exploration ID")
	trace.Flags().BoolVar(&traceJSON, "json", false, "emit JSON")
	trace.Flags().BoolVar(&traceMarkdown, "md", false, "emit Markdown")
	trace.MarkFlagsMutuallyExclusive("json", "md")
	trace.Flags().BoolVar(&traceReference, "output-reference", false, "emit the published View reference")
	_ = trace.Flags().MarkHidden("output-reference")
	_ = trace.MarkFlagRequired("exploration")
	var open bool
	var language string
	visual := &cobra.Command{Use: "visual", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, e := s.VisualView(cmd.Context(), open, language)
		return emit(out, r, e)
	}}
	visual.Flags().BoolVar(&open, "open", false, "open generated HTML")
	visual.Flags().StringVar(&language, "lang", "", "HTML language: zh or en (default: operating system language)")
	cmd.AddCommand(status, trace, visual)
	return cmd
}

func structuredViewFormat(asJSON, asMarkdown bool) app.ViewFormat {
	if asMarkdown {
		return app.ViewMarkdown
	}
	if asJSON {
		return app.ViewJSON
	}
	return app.ViewYAML
}

func emitStructuredView(out io.Writer, content []byte, path string, format app.ViewFormat, forceReference bool) error {
	if !forceReference && len(content) <= structuredViewInlineLimit {
		_, err := out.Write(content)
		return err
	}
	note := "The complete View has been written to the file at path; a file reference was requested."
	if len(content) > structuredViewInlineLimit {
		note = "The View exceeds the inline output limit. The complete content has been written to the file at path instead of being returned directly."
	}
	return output.WriteSuccess(out, publishedView{Path: path, Format: string(format), SizeBytes: len(content), Note: i18n.Message(note)}, nil, "View written to file")
}

func versionCommand(out io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		if asJSON {
			return output.JSON(out, struct {
				ProductVersion  string `json:"product_version"`
				Commit          string `json:"commit"`
				BuildTime       string `json:"build_time"`
				ProtocolVersion int    `json:"protocol_version"`
				SchemaVersion   int    `json:"schema_version"`
			}{version.ProductVersion, version.Commit, version.BuildTime, version.ProtocolVersion, version.SchemaVersion})
		}
		_, e := fmt.Fprintf(out, "norns %s (protocol %d, schema %d)\n", version.ProductVersion, version.ProtocolVersion, version.SchemaVersion)
		return e
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func emit(w io.Writer, r app.Result, err error) error {
	if err != nil {
		return err
	}
	return output.WriteSuccess(w, r.Data, r.Warnings, r.Message)
}
func normalizeCLIError(err error) error {
	var de *domain.Error
	if errors.As(err, &de) {
		return err
	}
	if strings.Contains(err.Error(), "required flag") || strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "arg(s)") || strings.Contains(err.Error(), "none of the others can be") {
		return domain.NewError("INVALID_ARGUMENT", err.Error())
	}
	return err
}

func Context() context.Context { return context.Background() }
func ExecutableDir() string    { p, _ := os.Executable(); return filepath.Dir(p) }
