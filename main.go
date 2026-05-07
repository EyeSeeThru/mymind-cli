package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
		"io"

	"mymind/pkg/auth"
	"mymind/pkg/client"
	"mymind/pkg/errors"
	"mymind/pkg/http"
	"mymind/pkg/output"
)

// Global flags
var (
	flagJSON     bool
	flagJSONL    bool
	flagNoColor  bool
	flagDryRun   bool
	flagVerbose  bool
)

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeNamedPipe == 0
}

func main() {
	root := &cobra.Command{Use: "mymind", Short: "MyMind CLI"}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output JSON")
	root.PersistentFlags().BoolVar(&flagJSONL, "jsonl", false, "Output JSONL (one object per line)")
	root.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Preview request without sending")
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose HTTP output")

	output.InitColors(!flagNoColor)

	root.AddCommand(authCmd())
	root.AddCommand(objectsCmd())
	root.AddCommand(spacesCmd())
	root.AddCommand(tagsCmd())
	root.AddCommand(searchCmd())
	root.AddCommand(convertCmd())

	root.SilenceErrors = true
	root.SilenceUsage = true

	if err := root.Execute(); err != nil {
		code := errors.MapExitCode(err)
		if flagJSON || flagJSONL {
			errObj := errors.ToJSON(err)
			enc := json.NewEncoder(os.Stderr)
			enc.SetIndent("", "  ")
			enc.Encode(errObj)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(code)
	}
}

// clientFromAuth returns a client. Requires real credentials unless --dry-run is set.
func clientFromAuth() (*client.Client, error) {
	creds, err := auth.Load()
	if err != nil {
		return nil, err
	}
	if creds == nil && !flagDryRun {
		return nil, fmt.Errorf("not authenticated. Run 'mymind auth login' or set MYMIND_KID and MYMIND_SECRET")
	}
	c := client.New(creds)
	c.SetVerbose(flagVerbose)
	return c, nil
}

func requestOpts() http.RequestOptions {
	return http.RequestOptions{DryRun: flagDryRun}
}

func printResult(data interface{}, formatter func(interface{}, output.Opts), opts output.Opts) {
	if flagJSON || flagJSONL {
		output.Print(data, opts)
		return
	}
	if formatter != nil {
		formatter(data, opts)
	} else {
		output.Print(data, opts)
	}
}

func getOpts() output.Opts {
	return output.Opts{JSON: flagJSON, JSONL: flagJSONL, NoColor: flagNoColor}
}

// Cobra command builders for each subcommand group
func objectsCmd() *cobra.Command {
	obj := &cobra.Command{Use: "objects", Short: "Objects (saved items)"}
	obj.AddCommand(objectsListCmd())
	obj.AddCommand(objectsGetCmd())
	obj.AddCommand(objectsCreateCmd())
	obj.AddCommand(objectsUpdateCmd())
	obj.AddCommand(objectsDeleteCmd())
	obj.AddCommand(objectsRestoreCmd())
	obj.AddCommand(objectsPinCmd())
	obj.AddCommand(objectsUnpinCmd())
	obj.AddCommand(objectsGetContentCmd())
	obj.AddCommand(objectsSetContentCmd())
	obj.AddCommand(objectsBlobCmd())
	obj.AddCommand(objectsScreenshotCmd())
	obj.AddCommand(objectsThumbnailCmd())
	obj.AddCommand(objectsTagCmd())
	obj.AddCommand(objectsNotesCmd())
	return obj
}

func spacesCmd() *cobra.Command {
	sp := &cobra.Command{Use: "spaces", Short: "Spaces (collections)"}
	sp.AddCommand(spacesListCmd())
	sp.AddCommand(spacesGetCmd())
	sp.AddCommand(spacesCreateCmd())
	sp.AddCommand(spacesUpdateCmd())
	sp.AddCommand(spacesDeleteCmd())
	sp.AddCommand(spacesAttachCmd())
	sp.AddCommand(spacesDetachCmd())
	return sp
}

func tagsCmd() *cobra.Command {
	tags := &cobra.Command{Use: "tags", Short: "Tags"}
	tags.AddCommand(&cobra.Command{
		Use: "list", Short: "List all tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil { return err }
			limit, _ := cmd.Flags().GetInt("limit")
			result, err := c.ListTags(limit, requestOpts())
			if err != nil { return err }
			printResult(result, output.PrintTags, getOpts())
			return nil
		},
	})
	tags.AddCommand(&cobra.Command{
		Use: "get <name>", Short: "Get tag details",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil { return err }
			result, err := c.GetTag(args[0], requestOpts())
			if err != nil { return err }
			printResult(result, output.PrintTags, getOpts())
			return nil
		},
	})
	tags.AddCommand(&cobra.Command{
		Use: "create <name>", Short: "Create a tag",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil { return err }
			result, err := c.CreateTag(args[0], requestOpts())
			if err != nil { return err }
			printResult(result, output.PrintTags, getOpts())
			return nil
		},
	})
	tags.AddCommand(&cobra.Command{
		Use: "delete <name>", Short: "Delete a tag",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil { return err }
			_, err = c.DeleteTag(args[0], requestOpts())
			if err != nil { return err }
			output.PrintOK("Tag deleted", getOpts())
			return nil
		},
	})
	tags.PersistentFlags().IntP("limit", "l", 20, "Max results")
	return tags
}

func searchCmd() *cobra.Command {
	sr := &cobra.Command{Use: "search [query words...]", Short: "Search objects", Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			limit, _ := cmd.Flags().GetInt("limit")
			semantic, _ := cmd.Flags().GetBool("semantic")
			rerank, _ := cmd.Flags().GetBool("rerank")
			similarTo, _ := cmd.Flags().GetString("similar-to")
			semanticBoost, _ := cmd.Flags().GetInt("semantic-boost")

			searchArgs := map[string]interface{}{
				"q":      strings.Join(args, " "),
				"limit":  limit,
			}
			if semantic {
				searchArgs["semantic"] = true
			}
			if rerank {
				searchArgs["rerank"] = true
			}
			if similarTo != "" {
				searchArgs["similarTo"] = similarTo
			}
			if semanticBoost > 0 {
				searchArgs["semanticBoost"] = semanticBoost
			}

			result, err := c.Search(searchArgs, requestOpts())
			if err != nil {
				return err
			}
			printResult(result, output.PrintSearch, getOpts())
			return nil
		},
	}
	sr.Flags().IntP("limit", "l", 20, "Max results")
	sr.Flags().Bool("semantic", false, "Enable semantic matching")
	sr.Flags().Int("semantic-boost", 0, "Semantic boost multiplier")
	sr.Flags().String("similar-to", "", "Find objects similar to given ID")
	sr.Flags().Bool("rerank", false, "Cross-encoder re-scoring")
	return sr
}

func convertCmd() *cobra.Command {
	cv := &cobra.Command{Use: "convert", Short: "Convert between text/markdown/prose formats"}
	cv.Flags().String("from", "", "Source format (text|markdown|prose)")
	cv.Flags().String("to", "", "Target format (text|markdown|prose)")
	cv.Flags().String("file", "", "Read input from file")
	cv.Flags().Bool("stdin", false, "Read input from stdin")
	cv.Flags().String("content", "", "Pass input inline")
	cv.MarkFlagRequired("from")
	cv.MarkFlagRequired("to")
	cv.RunE = func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		input, _ := cmd.Flags().GetString("content")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		filePath, _ := cmd.Flags().GetString("file")
		opts := getOpts()

		if useStdin {
			var buf []byte
			fmt.Scanln(&buf)
			input = string(buf)
		} else if filePath != "" {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			input = string(data)
		}

		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		result, err := c.Convert(input, from, to, requestOpts())
		if err != nil {
			return err
		}
		printResult(result, nil, opts)
		return nil
	}
	return cv
}

// ─── Auth command ─────────────────────────────────────────────────────────────

func authCmd() *cobra.Command {
	authCmd := &cobra.Command{Use: "auth", Short: "Authentication"}
	authCmd.AddCommand(&cobra.Command{Use: "login", Short: "Login with access key",
		RunE: func(cmd *cobra.Command, args []string) error {
			kid, _ := cmd.Flags().GetString("kid")
			secret, _ := cmd.Flags().GetString("secret")
			if kid == "" || secret == "" {
				fmt.Print("kid: ")
				fmt.Scanln(&kid)
				fmt.Print("secret: ")
				fmt.Scanln(&secret)
			}
			if kid == "" || secret == "" {
				return &errors.UserError{Message: "kid and secret are required"}
			}
			creds := &auth.Credentials{Kid: kid, Secret: secret}
			if err := auth.Save(creds); err != nil {
				return fmt.Errorf("saving credentials: %w", err)
			}
			output.PrintOK("Authenticated. Credentials saved to ~/.config/mymind/config.json", getOpts())
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{Use: "status", Short: "Check auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := auth.Load()
			if err != nil {
				return &errors.AuthError{Message: err.Error()}
			}
			output.PrintOK("Authenticated", getOpts())
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{Use: "logout", Short: "Remove stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.Clear(); err != nil {
				return fmt.Errorf("clearing credentials: %w", err)
			}
			output.PrintOK("Logged out", getOpts())
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{Use: "whoami", Short: "Verify credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			result, err := c.AuthProbe(requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Authenticated and verified", getOpts())
			_ = result
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{Use: "whoami", Short: "Alias for auth whoami",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.AuthProbe(requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Authenticated", getOpts())
			return nil
		},
	})

	authCmd.PersistentFlags().String("kid", "", "Access key ID")
	authCmd.PersistentFlags().String("secret", "", "Access key secret")
	return authCmd
}

// ─── Objects subcommands ───────────────────────────────────────────────────────

func objectsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List objects",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, _ := cmd.Flags().GetString("query")
			space, _ := cmd.Flags().GetString("space")
			ids, _ := cmd.Flags().GetString("ids")
			limit, _ := cmd.Flags().GetInt("limit")
			contentAs, _ := cmd.Flags().GetString("content-as")

			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			argsMap := map[string]interface{}{}
			if q != "" {
				argsMap["q"] = q
			}
			if space != "" {
				argsMap["spaceId"] = space
			}
			if ids != "" {
				idsList := strings.Split(ids, ",")
				argsMap["ids"] = idsList
			}
			if limit > 0 {
				argsMap["limit"] = limit
			}
			if contentAs != "" {
				argsMap["contentAs"] = contentAs
			}
			result, err := c.ListObjects(argsMap, requestOpts())
			if err != nil {
				return err
			}
			printResult(result, output.PrintObjectList, getOpts())
			return nil
		},
	}
}

func objectsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get one object by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			contentAs, _ := cmd.Flags().GetString("content-as")
			result, err := c.GetObject(args[0], map[string]interface{}{"contentAs": contentAs}, requestOpts())
			if err != nil {
				return err
			}
			printResult(result, output.PrintObjectDetail, getOpts())
			return nil
		},
	}
}

func objectsCreateCmd() *cobra.Command {
	create := &cobra.Command{Use: "create", Short: "Create an object"}
	create.Flags().String("url", "", "Save a URL")
	create.Flags().String("content", "", "Save text content")
	create.Flags().String("file", "", "Upload a file")
	create.Flags().Bool("stdin", false, "Read content from stdin")
	create.Flags().String("title", "", "Object title")
	create.Flags().StringArray("tag", []string{}, "Apply a tag")
	create.Flags().StringArray("space", []string{}, "Add to a space")
	create.RunE = func(cmd *cobra.Command, args []string) error {
		url, _ := cmd.Flags().GetString("url")
		content, _ := cmd.Flags().GetString("content")
		file, _ := cmd.Flags().GetString("file")
		stdin, _ := cmd.Flags().GetBool("stdin")
		title, _ := cmd.Flags().GetString("title")
		tags, _ := cmd.Flags().GetStringArray("tag")
		spaces, _ := cmd.Flags().GetStringArray("space")

		sources := []string{url, content, file}
		if stdin {
			sources = append(sources, "stdin")
		}
		provided := 0
		for _, s := range sources {
			if s != "" {
				provided++
			}
		}
		if provided == 0 {
			return &errors.UserError{Message: "provide one of --url, --content, --file, or --stdin"}
		}
		if provided > 1 {
			return &errors.UserError{Message: "provide exactly one of --url, --content, --file, or --stdin"}
		}

		c, err := clientFromAuth()
		if err != nil {
			return err
		}

		objArgs := map[string]interface{}{}
		if title != "" {
			objArgs["title"] = title
		}
		if len(tags) > 0 {
			objArgs["tags"] = tags
		}
		if len(spaces) > 0 {
			objArgs["spaces"] = spaces
		}

		if url != "" {
			objArgs["url"] = url
		} else if content != "" {
			objArgs["content"] = content
		} else if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			objArgs["blob"] = data
			objArgs["filename"] = file
		} else if stdin {
			var buf []byte
			fmt.Scanln(&buf)
			objArgs["content"] = string(buf)
		}

		result, err := c.CreateObject(objArgs, requestOpts())
		if err != nil {
			return err
		}
		printResult(result, output.PrintObjectDetail, getOpts())
		return nil
	}
	return create
}

func objectsUpdateCmd() *cobra.Command {
	update := &cobra.Command{
		Use:   "update <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update title or summary",
	}
	update.Flags().String("title", "", "New title")
	update.Flags().String("summary", "", "New summary")
	update.RunE = func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		summary, _ := cmd.Flags().GetString("summary")
		if title == "" && summary == "" {
			return &errors.UserError{Message: "provide --title or --summary"}
		}
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		result, err := c.UpdateObject(args[0], map[string]interface{}{"title": title, "summary": summary}, requestOpts())
		if err != nil {
			return err
		}
		if flagDryRun {
			printResult(result, nil, getOpts())
		} else {
			output.PrintOK("Object "+args[0]+" updated", getOpts())
		}
		_ = result
		return nil
	}
	return update
}

func objectsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Soft-delete an object",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.DeleteObject(args[0], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Object "+args[0]+" deleted", getOpts())
			return nil
		},
	}
}

func objectsRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Restore a soft-deleted object",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.RestoreObject(args[0], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Object "+args[0]+" restored", getOpts())
			return nil
		},
	}
}

func objectsPinCmd() *cobra.Command {
	pin := &cobra.Command{
		Use:   "pin <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Pin an object",
	}
	pin.Flags().Int("position", 0, "Zero-based slot index")
	pin.RunE = func(cmd *cobra.Command, args []string) error {
		pos, _ := cmd.Flags().GetInt("position")
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		_, err = c.PinObject(args[0], map[string]interface{}{"position": pos}, requestOpts())
		if err != nil {
			return err
		}
		output.PrintOK("Object "+args[0]+" pinned", getOpts())
		return nil
	}
	return pin
}

func objectsUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Unpin an object",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.UnpinObject(args[0], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Object "+args[0]+" unpinned", getOpts())
			return nil
		},
	}
}

func objectsGetContentCmd() *cobra.Command {
	gc := &cobra.Command{
		Use:   "get-content <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Fetch content body",
	}
	gc.Flags().StringP("format", "f", "markdown", "Content format (markdown|html|prose)")
	gc.RunE = func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		result, err := c.GetObjectContent(args[0], format, requestOpts())
		if err != nil {
			return err
		}
		if flagDryRun {
			printResult(result, nil, getOpts())
			return nil
		}
		if flagJSON || flagJSONL {
			printResult(map[string]interface{}{"id": args[0], "format": format, "content": result}, nil, getOpts())
		} else {
			fmt.Println(result)
		}
		return nil
	}
	return gc
}

func objectsSetContentCmd() *cobra.Command {
	sc := &cobra.Command{
		Use:   "set-content <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Replace content body",
	}
	sc.Flags().String("file", "", "Read body from file")
	sc.Flags().Bool("stdin", false, "Read body from stdin")
	sc.Flags().String("content", "", "Pass body inline")
	sc.Flags().StringP("format", "f", "markdown", "Content format (markdown|prose)")
	sc.RunE = func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		stdin, _ := cmd.Flags().GetBool("stdin")
		content, _ := cmd.Flags().GetString("content")
		format, _ := cmd.Flags().GetString("format")

		body := ""
		if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			body = string(data)
		} else if stdin {
			var buf []byte
			fmt.Scanln(&buf)
			body = string(buf)
		} else if content != "" {
			body = content
		} else {
			return &errors.UserError{Message: "provide --file, --stdin, or --content"}
		}

		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		_, err = c.SetObjectContent(args[0], body, format, requestOpts())
		if err != nil {
			return err
		}
		output.PrintOK("Object "+args[0]+" content updated", getOpts())
		return nil
	}
	return sc
}

func objectsBlobCmd() *cobra.Command {
	blob := &cobra.Command{
		Use:   "blob <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Stream the original file (binary)",
	}
	blob.Flags().StringP("output", "o", "", "Write to file")
	blob.RunE = func(cmd *cobra.Command, args []string) error {
		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" && !flagDryRun && isTerminal(os.Stdout) {
			return &errors.UserError{Message: "refusing to dump binary to TTY. Pass --output <path> or pipe to a file."}
		}
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		dryRun, resp, err := c.GetObjectBlob(args[0], requestOpts())
		if err != nil {
			return err
		}
		if dryRun != nil {
			printResult(*dryRun, nil, getOpts())
			return nil
		}
		if outputPath != "" {
			if err := client.StreamToFile(resp.Body, outputPath); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			output.PrintOK("wrote "+outputPath, getOpts())
		} else {
			io.Copy(os.Stdout, resp.Body)
		}
		resp.Body.Close()
		return nil
	}
	return blob
}

func objectsScreenshotCmd() *cobra.Command {
	ss := &cobra.Command{
		Use:   "screenshot <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Stream the rendered screenshot",
	}
	ss.Flags().StringP("output", "o", "", "Write to file")
	ss.RunE = func(cmd *cobra.Command, args []string) error {
		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" && !flagDryRun && isTerminal(os.Stdout) {
			return &errors.UserError{Message: "refusing to dump binary to TTY. Pass --output <path> or pipe to a file."}
		}
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		dryRun, resp, err := c.GetObjectScreenshot(args[0], requestOpts())
		if err != nil {
			return err
		}
		if dryRun != nil {
			printResult(*dryRun, nil, getOpts())
			return nil
		}
		if outputPath != "" {
			if err := client.StreamToFile(resp.Body, outputPath); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			output.PrintOK("wrote "+outputPath, getOpts())
		} else {
			io.Copy(os.Stdout, resp.Body)
		}
		resp.Body.Close()
		return nil
	}
	return ss
}

func objectsThumbnailCmd() *cobra.Command {
	th := &cobra.Command{
		Use:   "thumbnail <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Stream a thumbnail",
	}
	th.Flags().String("size", "", "Containment box (e.g. 256x256)")
	th.Flags().StringP("output", "o", "", "Write to file")
	th.RunE = func(cmd *cobra.Command, args []string) error {
		size, _ := cmd.Flags().GetString("size")
		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" && !flagDryRun && isTerminal(os.Stdout) {
			return &errors.UserError{Message: "refusing to dump binary to TTY. Pass --output <path> or pipe to a file."}
		}
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		dryRun, resp, err := c.GetObjectThumbnail(args[0], size, requestOpts())
		if err != nil {
			return err
		}
		if dryRun != nil {
			printResult(*dryRun, nil, getOpts())
			return nil
		}
		if outputPath != "" {
			if err := client.StreamToFile(resp.Body, outputPath); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			output.PrintOK("wrote "+outputPath, getOpts())
		} else {
			io.Copy(os.Stdout, resp.Body)
		}
		resp.Body.Close()
		return nil
	}
	return th
}

func objectsTagCmd() *cobra.Command {
	tag := &cobra.Command{Use: "tag", Short: "Tag operations"}
	tag.AddCommand(&cobra.Command{
		Use:   "add <id> <tags...>",
		Args:  cobra.MinimumNArgs(2),
		Short: "Apply tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			names := args[1:]
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.AddObjectTags(id, names, requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Tags added to "+id+": "+strings.Join(names, ", "), getOpts())
			return nil
		},
	})
	tag.AddCommand(&cobra.Command{
		Use:   "remove <id> <tags...>",
		Args:  cobra.MinimumNArgs(2),
		Short: "Remove tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			names := args[1:]
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.RemoveObjectTags(id, names, requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Tags removed from "+id+": "+strings.Join(names, ", "), getOpts())
			return nil
		},
	})
	return tag
}

func objectsNotesCmd() *cobra.Command {
	notes := &cobra.Command{Use: "notes", Short: "Notes on objects"}
	notes.AddCommand(&cobra.Command{
		Use:   "list <id>",
		Args:  cobra.ExactArgs(1),
		Short: "List notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			result, err := c.GetObject(args[0], map[string]interface{}{}, requestOpts())
			if err != nil {
				return err
			}
			// Notes are embedded in the object
			printResult(result, output.PrintObjectDetail, getOpts())
			return nil
		},
	})
	notes.AddCommand(&cobra.Command{
		Use:   "add <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Add a note",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, _ := cmd.Flags().GetString("content")
			format, _ := cmd.Flags().GetString("format")
			if content == "" {
				return &errors.UserError{Message: "provide --content"}
			}
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			result, err := c.AddObjectNote(args[0], content, format, requestOpts())
			if err != nil {
				return err
			}
			printResult(result, nil, getOpts())
			return nil
		},
	})
	notes.AddCommand(&cobra.Command{
		Use:   "update <id> <note-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Update a note",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, _ := cmd.Flags().GetString("content")
			format, _ := cmd.Flags().GetString("format")
			if content == "" {
				return &errors.UserError{Message: "provide --content"}
			}
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.UpdateObjectNote(args[0], args[1], content, format, requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Note "+args[1]+" on "+args[0]+" updated", getOpts())
			return nil
		},
	})
	notes.AddCommand(&cobra.Command{
		Use:   "delete <id> <note-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Delete a note",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.DeleteObjectNote(args[0], args[1], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Note "+args[1]+" on "+args[0]+" deleted", getOpts())
			return nil
		},
	})

	notes.PersistentFlags().String("content", "", "Note body")
	notes.PersistentFlags().StringP("format", "f", "markdown", "Content format (markdown|prose)")
	return notes
}

// ─── Spaces subcommands ───────────────────────────────────────────────────────

func spacesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List spaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			result, err := c.ListSpaces(requestOpts())
			if err != nil {
				return err
			}
			printResult(result, output.PrintSpaces, getOpts())
			return nil
		},
	}
}

func spacesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a space",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			result, err := c.GetSpace(args[0], requestOpts())
			if err != nil {
				return err
			}
			printResult(result, output.PrintSpaces, getOpts())
			return nil
		},
	}
}

func spacesCreateCmd() *cobra.Command {
	sp := &cobra.Command{
		Use:   "create",
		Short: "Create a space",
	}
	sp.Flags().String("name", "", "Space name")
	sp.Flags().String("color", "", "Space color")
	sp.MarkFlagRequired("name")
	sp.RunE = func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		color, _ := cmd.Flags().GetString("color")
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		result, err := c.CreateSpace(map[string]interface{}{"name": name, "color": color}, requestOpts())
		if err != nil {
			return err
		}
		printResult(result, output.PrintSpaces, getOpts())
		return nil
	}
	return sp
}

func spacesUpdateCmd() *cobra.Command {
	sp := &cobra.Command{
		Use:   "update <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Rename or recolor a space",
	}
	sp.Flags().String("name", "", "New name")
	sp.Flags().String("color", "", "New color")
	sp.RunE = func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		color, _ := cmd.Flags().GetString("color")
		if name == "" && color == "" {
			return &errors.UserError{Message: "provide --name or --color"}
		}
		c, err := clientFromAuth()
		if err != nil {
			return err
		}
		result, err := c.UpdateSpace(args[0], map[string]interface{}{"name": name, "color": color}, requestOpts())
		if err != nil {
			return err
		}
		printResult(result, output.PrintSpaces, getOpts())
		return nil
	}
	return sp
}

func spacesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a space",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.DeleteSpace(args[0], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Space "+args[0]+" deleted", getOpts())
			return nil
		},
	}
}

func spacesAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <space-id> <object-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Add an object to a space",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.AttachToSpace(args[0], args[1], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Object "+args[1]+" added to space "+args[0], getOpts())
			return nil
		},
	}
}

func spacesDetachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detach <space-id> <object-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Remove an object from a space",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromAuth()
			if err != nil {
				return err
			}
			_, err = c.DetachFromSpace(args[0], args[1], requestOpts())
			if err != nil {
				return err
			}
			output.PrintOK("Object "+args[1]+" removed from space "+args[0], getOpts())
			return nil
		},
	}
}
