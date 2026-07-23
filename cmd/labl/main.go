// labl is the labl-printr command line: fire labels at the printer from any
// machine on the LAN. Modeled on the mmb CLI's ergonomics — kebab-case
// subcommands, explicit flags, --dry-run, idempotency keys.
package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var stdout = tabbed(os.Stdout)

func tabbed(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
}

func main() {
	root := &cobra.Command{
		Use:           "labl",
		Short:         "Print labels via labl-printr",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(cmdConfig(), cmdTemplates(), cmdPrint(), cmdPreview(), cmdRaw(),
		cmdPrinters(), cmdDiscover(), cmdStatus(), cmdJobs(), cmdReprint())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "labl:", err)
		os.Exit(1)
	}
}

func cmdConfig() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show or set the server address"}
	cmd.AddCommand(&cobra.Command{
		Use: "show", Short: "Show current config",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := loadConfig()
			fmt.Printf("server: %s\nconfig: %s\n", c.Server, configPath())
			return nil
		},
	}, &cobra.Command{
		Use: "set-server <url>", Short: "Point labl at a labl-printr server", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := loadConfig()
			c.Server = strings.TrimRight(args[0], "/")
			if err := saveConfig(c); err != nil {
				return err
			}
			fmt.Printf("server set to %s\n", c.Server)
			return nil
		},
	})
	return cmd
}

func cmdTemplates() *cobra.Command {
	return &cobra.Command{
		Use: "templates", Short: "List available label templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			var list []templateInfo
			if err := newClient().do("GET", "/api/templates", nil, &list); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "ID\tNAME\tFIELDS\tKIND")
			for _, t := range list {
				var keys []string
				for _, f := range t.Fields {
					k := f.Key
					if f.Required {
						k += "*"
					}
					keys = append(keys, k)
				}
				kind := "custom"
				if t.Builtin {
					kind = "built-in"
				}
				fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", t.ID, t.Name, strings.Join(keys, ","), kind)
			}
			return stdout.Flush()
		},
	}
}

type printFlags struct {
	vars           []string
	printer        string
	copies         int
	dryRun         bool
	idempotencyKey string
	out            string
}

func parseVars(pairs []string) (map[string]string, error) {
	vars := map[string]string{}
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--var wants key=value, got %q", pair)
		}
		vars[k] = v
	}
	return vars, nil
}

func cmdPrint() *cobra.Command {
	f := printFlags{}
	cmd := &cobra.Command{
		Use:   "print <template>",
		Short: "Print a label from a template",
		Long:  "Print a label from a template.\n\nExample:\n  labl print inventory --var name=\"M3 screws\" --var sku=HW-M3-012 --var url=https://inv.local/123\n  labl print large-print --var text=FRAGILE --copies 3",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseVars(f.vars)
			if err != nil {
				return err
			}
			body := map[string]any{
				"templateId": args[0], "vars": vars, "copies": f.copies,
				"printer": f.printer, "source": "cli", "idempotencyKey": f.idempotencyKey,
			}
			if f.dryRun {
				var res previewResult
				if err := newClient().do("POST", "/api/preview", body, &res); err != nil {
					return err
				}
				fmt.Printf("dry run — label renders %d×%d dots (%.1f×%.2f in). Nothing printed.\n",
					res.WidthDots, res.LengthDots, float64(res.WidthDots)/203, float64(res.LengthDots)/203)
				return nil
			}
			var job jobInfo
			if err := newClient().do("POST", "/api/jobs", body, &job); err != nil {
				return err
			}
			fmt.Printf("job %d queued on %s (%d cop%s)\n", job.ID, job.PrinterName, job.Copies, plural(job.Copies, "y", "ies"))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&f.vars, "var", nil, "template variable key=value (repeatable)")
	cmd.Flags().StringVar(&f.printer, "printer", "", "printer name (default: server's default printer)")
	cmd.Flags().IntVar(&f.copies, "copies", 1, "number of copies")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "validate and size the label without printing")
	cmd.Flags().StringVar(&f.idempotencyKey, "idempotency-key", "", "safe-retry key: same key never prints twice")
	return cmd
}

func cmdPreview() *cobra.Command {
	f := printFlags{}
	cmd := &cobra.Command{
		Use:   "preview <template>",
		Short: "Render a label to a PNG without printing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseVars(f.vars)
			if err != nil {
				return err
			}
			var res previewResult
			body := map[string]any{"templateId": args[0], "vars": vars, "printer": f.printer}
			if err := newClient().do("POST", "/api/preview", body, &res); err != nil {
				return err
			}
			png, err := base64.StdEncoding.DecodeString(res.PNG)
			if err != nil {
				return err
			}
			if err := os.WriteFile(f.out, png, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s (%d×%d dots)\n", f.out, res.WidthDots, res.LengthDots)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&f.vars, "var", nil, "template variable key=value (repeatable)")
	cmd.Flags().StringVar(&f.printer, "printer", "", "printer whose geometry to render for")
	cmd.Flags().StringVarP(&f.out, "out", "o", "label.png", "output PNG path")
	return cmd
}

func cmdRaw() *cobra.Command {
	f := printFlags{}
	cmd := &cobra.Command{
		Use:   "raw [file]",
		Short: "Print raw ZPL from a file or stdin",
		Long:  "Print raw ZPL from a file or stdin.\n\nExample:\n  cat label.zpl | labl raw\n  labl raw label.zpl --printer zd421",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var data []byte
			var err error
			if len(args) == 1 && args[0] != "-" {
				data, err = os.ReadFile(args[0])
			} else {
				data, err = io.ReadAll(os.Stdin)
			}
			if err != nil {
				return err
			}
			if len(strings.TrimSpace(string(data))) == 0 {
				return fmt.Errorf("no ZPL on stdin — pipe a file or pass a path")
			}
			body := map[string]any{"zpl": string(data), "printer": f.printer, "source": "cli"}
			var job jobInfo
			if err := newClient().do("POST", "/api/jobs", body, &job); err != nil {
				return err
			}
			fmt.Printf("job %d queued on %s\n", job.ID, job.PrinterName)
			return nil
		},
	}
	cmd.Flags().StringVar(&f.printer, "printer", "", "printer name (default: server's default printer)")
	return cmd
}

func cmdPrinters() *cobra.Command {
	return &cobra.Command{
		Use: "printers", Short: "List printers",
		RunE: func(cmd *cobra.Command, args []string) error {
			var list []printerInfo
			if err := newClient().do("GET", "/api/printers", nil, &list); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "ID\tNAME\tKIND\tADDRESS\tDPMM\tDEFAULT")
			for _, p := range list {
				addr := "-"
				if p.Host != "" {
					addr = fmt.Sprintf("%s:%d", p.Host, p.Port)
				}
				def := ""
				if p.IsDefault {
					def = "✓"
				}
				fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\t%d\t%s\n", p.ID, p.Name, p.Kind, addr, p.Dpmm, def)
			}
			return stdout.Flush()
		},
	}
}

func cmdDiscover() *cobra.Command {
	return &cobra.Command{
		Use: "discover", Short: "Find Zebra printers on the LAN (UDP broadcast)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var found []struct {
				IP   string   `json:"ip"`
				Info []string `json:"info"`
			}
			if err := newClient().do("POST", "/api/printers/discover", nil, &found); err != nil {
				return err
			}
			if len(found) == 0 {
				fmt.Println("no printers answered — is the printer on the same subnet and powered up?")
				return nil
			}
			for _, d := range found {
				fmt.Printf("%s\t%s\n", d.IP, strings.Join(d.Info, " | "))
			}
			return nil
		},
	}
}

func cmdStatus() *cobra.Command {
	printerName := ""
	cmd := &cobra.Command{
		Use: "status", Short: "Live printer status (~HS decoded)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var printers []printerInfo
			if err := newClient().do("GET", "/api/printers", nil, &printers); err != nil {
				return err
			}
			for _, p := range printers {
				if printerName != "" && p.Name != printerName {
					continue
				}
				var st statusInfo
				if err := newClient().do("GET", fmt.Sprintf("/api/printers/%d/status", p.ID), nil, &st); err != nil {
					return err
				}
				state := "ready"
				if !st.Reachable {
					state = "unreachable"
				} else if !st.Ready {
					state = "not ready"
				}
				line := fmt.Sprintf("%s: %s", p.Name, state)
				if st.Detail != "" {
					line += " (" + st.Detail + ")"
				}
				if st.FormatsBuffered > 0 {
					line += fmt.Sprintf(" — %d format(s) still in buffer", st.FormatsBuffered)
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&printerName, "printer", "", "only this printer")
	return cmd
}

func cmdJobs() *cobra.Command {
	limit := 20
	cmd := &cobra.Command{
		Use: "jobs", Short: "Recent print jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			var list []jobInfo
			if err := newClient().do("GET", fmt.Sprintf("/api/jobs?limit=%d", limit), nil, &list); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "ID\tSTATE\tTEMPLATE\tPRINTER\tCOPIES\tWHEN\tERROR")
			for _, j := range list {
				tpl := j.TemplateID
				if tpl == "" {
					tpl = "(raw zpl)"
				}
				fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\t%d\t%s\t%s\n", j.ID, j.State, tpl, j.PrinterName, j.Copies, j.CreatedAt, j.Error)
			}
			return stdout.Flush()
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "how many jobs to show")
	return cmd
}

func cmdReprint() *cobra.Command {
	return &cobra.Command{
		Use: "reprint <job-id>", Short: "Print a past job again, byte-identical", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var job jobInfo
			if err := newClient().do("POST", "/api/jobs/"+args[0]+"/reprint", nil, &job); err != nil {
				return err
			}
			fmt.Printf("job %d queued on %s (reprint of %s)\n", job.ID, job.PrinterName, args[0])
			return nil
		},
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
