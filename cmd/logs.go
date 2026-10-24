package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/logging"
	"github.com/genzj/goTApaper/service"
	"github.com/genzj/goTApaper/util"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Read the recent log output",
	Long: `Read the recent log output.

Entries come from the running daemon when one is listening, so this shows what
the background process has been doing rather than only this invocation. With no
daemon running there is nothing in memory to read, and the rotated files listed
by "logs files" are the place to look.`,
	Run: func(cmd *cobra.Command, args []string) {
		if follow {
			followLogs()
			return
		}
		printLogs()
	},
}

var logsFilesCmd = &cobra.Command{
	Use:   "files",
	Short: "List the log files on disk",
	Run: func(cmd *cobra.Command, args []string) {
		printLogFiles()
	},
}

var (
	follow   bool
	logLevel string
	logLines int
)

func init() {
	logsCmd.PersistentFlags().BoolVarP(
		&follow, "follow", "f", false, "keep the connection open and print new entries as they arrive",
	)
	logsCmd.PersistentFlags().StringVar(
		&logLevel, "level", "", "show entries at this level or more severe",
	)
	logsCmd.PersistentFlags().IntVarP(
		&logLines, "lines", "n", 50, "number of entries to show",
	)

	logsCmd.AddCommand(logsFilesCmd)
	RootCmd.AddCommand(logsCmd)
}

// printLogs writes the buffered entries the daemon is holding.
func printLogs() {
	client := daemonClient()
	if client == nil {
		fmt.Fprintln(os.Stderr,
			"no daemon is running, so there is no log buffer to read; "+
				"see \"goTApaper logs files\" for the rotated files on disk")
		return
	}

	var page service.LogPage
	if err := client.Logs(logLevel, 0, logLines, &page); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, entry := range page.Entries {
		fmt.Println(formatLogEntry(entry))
	}
}

// followLogs prints buffered entries and then streams new ones.
//
// The snapshot comes first so "--follow" does not start on a blank screen
// while waiting for something to happen, which looks like a hang.
func followLogs() {
	client := daemonClient()
	if client == nil {
		fmt.Fprintln(os.Stderr, "no daemon is running, so there is nothing to follow")
		os.Exit(1)
	}

	var page service.LogPage
	if err := client.Logs(logLevel, 0, logLines, &page); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, entry := range page.Entries {
		fmt.Println(formatLogEntry(entry))
	}

	stream, err := client.Events()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer stream.Close()

	// The same stream the interface uses, so what the CLI tails and what the
	// UI shows cannot drift.
	scanner := bufio.NewScanner(stream)
	// Long debug lines carry whole configuration dumps, well past bufio's
	// 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventType string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if eventType != api.EventLog {
				continue
			}
			var entry logging.Entry
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &entry); err != nil {
				continue
			}
			if !matchesLevel(entry, logLevel) {
				continue
			}
			fmt.Println(formatLogEntry(entry))
		}
	}
}

// matchesLevel filters a streamed entry, since the stream itself carries every
// level: the server cannot filter per subscriber without one stream each.
func matchesLevel(entry logging.Entry, minLevel string) bool {
	if minLevel == "" {
		return true
	}
	return logging.Filter{MinLevel: minLevel}.Matches(entry)
}

// formatLogEntry renders one entry in the same shape as logrus' text output,
// so switching between the log file, stderr and this command does not mean
// re-learning the format.
func formatLogEntry(entry logging.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, `time="%s" level=%s msg=%q`,
		entry.Time.Local().Format("2006-01-02T15:04:05Z07:00"), entry.Level, entry.Message)

	// Sorted, so successive lines put the same field in the same place.
	keys := make([]string, 0, len(entry.Fields))
	for k := range entry.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%q", k, entry.Fields[k])
	}
	return b.String()
}

func printLogFiles() {
	files, err := logging.Files(config.GetLogFileName())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("no log files yet")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tACTIVE\tPATH")
	for _, f := range files {
		active := ""
		if f.Active {
			active = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Name, util.HumanBytes(f.Size), active, f.Path)
	}
	w.Flush()
}
