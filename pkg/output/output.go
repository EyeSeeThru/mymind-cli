package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rodaine/table"
)

// Mode is the output mode.
type Mode int

const (
	ModePretty Mode = iota
	ModeJSON
	ModeJSONL
)

// Opts controls output formatting.
type Opts struct {
	JSON    bool
	JSONL   bool
	NoColor bool
}

// GetMode returns the output mode from options and environment.
func GetMode(o Opts) Mode {
	if o.JSON || os.Getenv("MYMIND_JSON") == "1" {
		return ModeJSON
	}
	if o.JSONL {
		return ModeJSONL
	}
	return ModePretty
}

var (
	dim  = func(s string) string { return "\033[90m" + s + "\033[0m" }
	bold = func(s string) string { return "\033[1m" + s + "\033[0m" }
	grn  = func(s string) string { return "\033[32m" + s + "\033[0m" }
	ylw  = func(s string) string { return "\033[33m" + s + "\033[0m" }
	red  = func(s string) string { return "\033[31m" + s + "\033[0m" }
)

// InitColors enables ANSI color codes.
func InitColors(enabled bool) {
	if !enabled {
		dim = func(s string) string { return s }
		bold = func(s string) string { return s }
		grn = func(s string) string { return s }
		ylw = func(s string) string { return s }
		red = func(s string) string { return s }
	}
}

// IsDryRun checks if a value is a DryRunResult.
func IsDryRun(v interface{}) bool {
	_, ok := v.(DryRunResult)
	return ok
}

// DryRunResult holds the preview of a dry-run request.
type DryRunResult struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// Print outputs data according to the current mode.
func Print(data interface{}, opts Opts) {
	mode := GetMode(opts)

	if d, ok := data.(DryRunResult); ok {
		printDryRun(d, mode)
		return
	}

	switch mode {
	case ModeJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
	case ModeJSONL:
		if arr, ok := data.([]interface{}); ok {
			for _, item := range arr {
				b, _ := json.Marshal(item)
				os.Stdout.Write(append(b, '\n'))
			}
		} else if data != nil {
			b, _ := json.Marshal(data)
			os.Stdout.Write(append(b, '\n'))
		}
	default:
		fmt.Fprintln(os.Stdout, data)
	}
}

func printDryRun(d DryRunResult, mode Mode) {
	if mode == ModeJSON || mode == ModeJSONL {
		obj := map[string]interface{}{
			"dryRun": true,
			"method": d.Method,
			"url":    d.URL,
			"headers": d.Headers,
			"body":   d.Body,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(obj)
		return
	}
	fmt.Printf("%s %s\n", bold(ylw("DRY RUN — no request sent")))
	fmt.Printf("  %s %s %s\n", bold(d.Method), ylw("→"), d.URL)
	for k, v := range d.Headers {
		fmt.Printf("  %s %s\n", dim(k+":"), v)
	}
	if d.Body != "" {
		fmt.Printf("  %s %s\n", dim("body:"), d.Body)
	}
}

// PrintOK prints a success message.
func PrintOK(msg string, opts Opts) {
	mode := GetMode(opts)
	if mode == ModeJSON {
		fmt.Fprintf(os.Stdout, `{"ok":true,"message":%s}`+"\n", strconv.Quote(msg))
	} else if mode == ModeJSONL {
		fmt.Fprintf(os.Stdout, `{"ok":true,"message":%s}`+"\n", strconv.Quote(msg))
	} else {
		fmt.Printf("%s %s\n", grn("✓"), msg)
	}
}

// Trunc truncates a string to max length.
func Trunc(s string, max int) string {
	if s == "" {
		return ""
	}
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// FmtTime formats an ISO timestamp for display.
func FmtTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04")
}

// ─── Pretty formatters ─────────────────────────────────────────────────────────

// PrintObjectList prints objects as a table.
func PrintObjectList(data interface{}, opts Opts) {
	objects, ok := data.([]interface{})
	if !ok {
		Print(data, opts)
		return
	}

	tbl := table.New("id", "type", "title", "tags", "spaces", "bumped")
	tbl.WithWriter(os.Stdout)
	for _, item := range objects {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := str(obj["id"])
		objType := str(obj["type"])
		title := Trunc(str(obj["title"]), 50)
		if title == "" {
			title = Trunc(str(obj["url"]), 50)
		}
		tags := tagNames(obj["tags"])
		spaces := spaceNames(obj["spaces"])
		bumped := FmtTime(str(obj["bumped"]))
		if bumped == "" {
			bumped = FmtTime(str(obj["modified"]))
		}
		if bumped == "" {
			bumped = FmtTime(str(obj["created"]))
		}
		tbl.AddRow(id, objType, title, tags, spaces, bumped)
	}
	tbl.Print()
	fmt.Fprintf(os.Stdout, "(%d object%s)\n", len(objects), plural(len(objects)))
}

// PrintObjectDetail prints a single object as a key-value table.
func PrintObjectDetail(data interface{}, opts Opts) {
	obj, ok := data.(map[string]interface{})
	if !ok {
		Print(data, opts)
		return
	}

	tbl := table.New("field", "value")
	tbl.WithWriter(os.Stdout)

	kv := [][2]string{
		{"id", str(obj["id"])},
	}
	add := func(k, v string) {
		if v != "" {
			kv = append(kv, [2]string{k, v})
		}
	}
	add("type", str(obj["type"]))
	add("title", str(obj["title"]))
	add("url", Trunc(str(obj["url"]), 80))
	add("summary", Trunc(str(obj["summary"]), 100))
	add("tags", tagNames(obj["tags"]))
	add("spaces", spaceNames(obj["spaces"]))
	if notes, ok := obj["notes"].([]interface{}); ok {
		add("notes", fmt.Sprintf("%d", len(notes)))
	}
	add("created", FmtTime(str(obj["created"])))
	add("modified", FmtTime(str(obj["modified"])))
	add("bumped", FmtTime(str(obj["bumped"])))

	for _, row := range kv {
		tbl.AddRow(row[0], row[1])
	}
	tbl.Print()
}

// PrintSearch prints search results as a table.
func PrintSearch(data interface{}, opts Opts) {
	resp, ok := data.(map[string]interface{})
	if !ok {
		Print(data, opts)
		return
	}
	matches, _ := resp["matches"].([]interface{})
	if matches == nil {
		fmt.Fprintln(os.Stdout, "()")
		return
	}

	// Check if any have semanticScore
	hasSemantic := false
	for _, m := range matches {
		if mm, ok := m.(map[string]interface{}); ok {
			if _, ok := mm["semanticScore"]; ok {
				hasSemantic = true
				break
			}
		}
	}

	if hasSemantic {
		tbl := table.New("id", "score", "semantic")
		tbl.WithWriter(os.Stdout)
		for _, m := range matches {
			if mm, ok := m.(map[string]interface{}); ok {
				id := str(mm["id"])
				score := fmtScore(mm["score"])
				semantic := fmtScore(mm["semanticScore"])
				tbl.AddRow(id, score, semantic)
			}
		}
		tbl.Print()
	} else {
		tbl := table.New("id", "score")
		tbl.WithWriter(os.Stdout)
		for _, m := range matches {
			if mm, ok := m.(map[string]interface{}); ok {
				id := str(mm["id"])
				score := fmtScore(mm["score"])
				tbl.AddRow(id, score)
			}
		}
		tbl.Print()
	}
	fmt.Fprintf(os.Stdout, "(%d match%s)\n", len(matches), plural(len(matches)))
}

// PrintTags prints tags as a table.
func PrintTags(data interface{}, opts Opts) {
	tags, ok := data.([]interface{})
	if !ok {
		Print(data, opts)
		return
	}

	tbl := table.New("name", "count", "modified")
	tbl.WithWriter(os.Stdout)
	for _, item := range tags {
		if tag, ok := item.(map[string]interface{}); ok {
			name := str(tag["name"])
			count := ""
			if c, ok := tag["count"].(float64); ok {
				count = fmt.Sprintf("%.0f", c)
			}
			modified := FmtTime(str(tag["modified"]))
			tbl.AddRow(name, count, modified)
		}
	}
	tbl.Print()
	fmt.Fprintf(os.Stdout, "(%d tag%s)\n", len(tags), plural(len(tags)))
}

// PrintSpaces prints spaces as a table.
func PrintSpaces(data interface{}, opts Opts) {
	spaces, ok := data.([]interface{})
	if !ok {
		Print(data, opts)
		return
	}

	tbl := table.New("id", "name", "color")
	tbl.WithWriter(os.Stdout)
	for _, item := range spaces {
		if space, ok := item.(map[string]interface{}); ok {
			tbl.AddRow(str(space["id"]), str(space["name"]), str(space["color"]))
		}
	}
	tbl.Print()
	fmt.Fprintf(os.Stdout, "(%d space%s)\n", len(spaces), plural(len(spaces)))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func fmtScore(v interface{}) string {
	if v == nil {
		return ""
	}
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.3f", f)
	}
	return fmt.Sprintf("%v", v)
}

func tagNames(v interface{}) string {
	if v == nil {
		return ""
	}
	arr, ok := v.([]interface{})
	if !ok {
		return ""
	}
	var names []string
	for _, t := range arr {
		if m, ok := t.(map[string]interface{}); ok {
			if n, ok := m["name"].(string); ok && n != "" {
				names = append(names, n)
			}
		}
	}
	return strings.Join(names, ", ")
}

func spaceNames(v interface{}) string {
	if v == nil {
		return ""
	}
	arr, ok := v.([]interface{})
	if !ok {
		return ""
	}
	var names []string
	for _, s := range arr {
		if m, ok := s.(map[string]interface{}); ok {
			if n, ok := m["name"].(string); ok && n != "" {
				names = append(names, n)
			} else if id, ok := m["id"].(string); ok {
				names = append(names, id)
			}
		}
	}
	return strings.Join(names, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// StreamToFile copies a response body to a file.
func StreamToFile(r io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
