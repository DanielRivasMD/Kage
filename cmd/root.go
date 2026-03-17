/*
Copyright © 2026 Daniel Rivas <danielrivasmd@gmail.com>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

////////////////////////////////////////////////////////////////////////////////////////////////////

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/DanielRivasMD/domovoi"
	"github.com/DanielRivasMD/horus"
	"github.com/spf13/cobra"
)

////////////////////////////////////////////////////////////////////////////////////////////////////

//go:embed docs.json
var docsFS embed.FS

////////////////////////////////////////////////////////////////////////////////////////////////////

const (
	APP     = "kage"
	VERSION = "v0.1.0"
	AUTHOR  = "Daniel Rivas"
	EMAIL   = "<danielrivasmd@gmail.com>"
)

////////////////////////////////////////////////////////////////////////////////////////////////////

func InitDocs() {
	info := domovoi.AppInfo{
		Name:    APP,
		Version: VERSION,
		Author:  AUTHOR,
		Email:   EMAIL,
	}
	domovoi.SetGlobalDocsConfig(docsFS, info)
}

////////////////////////////////////////////////////////////////////////////////////////////////////

func GetRootCmd() *cobra.Command {
	onceRoot.Do(func() {
		d := horus.Must(domovoi.GlobalDocs())
		var err error
		rootCmd, err = d.MakeCmd("root", nil,
			domovoi.WithArgs(cobra.MinimumNArgs(1)),
		)
		horus.CheckErr(err)

		rootCmd.PersistentFlags().BoolVarP(&rootFlags.verbose, "verbose", "v", false, "Enable verbose diagnostics")
		rootCmd.PersistentFlags().BoolVarP(&rootFlags.copyOut, "out", "o", false, "Copy stdout to clipboard")
		rootCmd.PersistentFlags().BoolVarP(&rootFlags.copyErr, "err", "e", false, "Copy stderr to clipboard")
		rootCmd.DisableFlagParsing = true
		rootCmd.Version = VERSION

		rootCmd.Run = runRoot
	})
	return rootCmd
}

////////////////////////////////////////////////////////////////////////////////////////////////////

func Execute() {
	horus.CheckErr(GetRootCmd().Execute())
}

////////////////////////////////////////////////////////////////////////////////////////////////////

type rootFlag struct {
	verbose bool
	copyOut bool
	copyErr bool
}

var (
	onceRoot  sync.Once
	rootCmd   *cobra.Command
	rootFlags rootFlag
)

////////////////////////////////////////////////////////////////////////////////////////////////////

func BuildCommands() {
	root := GetRootCmd()
	root.AddCommand(
		CompletionCmd(),
		IdentityCmd(),
	)
}

////////////////////////////////////////////////////////////////////////////////////////////////////

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI color escape sequences from a string.
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

////////////////////////////////////////////////////////////////////////////////////////////////////

func copyToClipboard(data []byte, verbose bool) {
	if len(data) == 0 {
		if verbose {
			fmt.Fprintln(os.Stderr, "No data to copy to clipboard")
		}
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("copyq", "copy", "-")
	default:
		if verbose {
			fmt.Fprintf(os.Stderr, "Clipboard copy not supported on %s\n", runtime.GOOS)
		}
		return
	}

	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to copy to clipboard: %v\n", err)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////

func runRoot(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		cmd.Help()
		os.Exit(0)
	}

	filteredArgs := []string{}
	verbose := false
	copyOut := false
	copyErr := false
	stopParsing := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if stopParsing {
			filteredArgs = append(filteredArgs, arg)
			continue
		}

		switch arg {
		case "--":
			stopParsing = true
		case "-v", "--verbose":
			verbose = true
		case "-o", "--out":
			copyOut = true
		case "-e", "--err":
			copyErr = true
		case "-h", "--help":
			cmd.Help()
			os.Exit(0)
		case "--version":
			fmt.Println(cmd.Version)
			os.Exit(0)
		default:
			filteredArgs = append(filteredArgs, arg)
		}
	}

	rootFlags.verbose = verbose
	rootFlags.copyOut = copyOut
	rootFlags.copyErr = copyErr

	if len(filteredArgs) == 0 {
		cmd.Help()
		os.Exit(1)
	}

	command := filteredArgs[0]
	commandArgs := filteredArgs[1:]

	var stdoutBuf, stderrBuf bytes.Buffer
	c := exec.Command(command, commandArgs...)
	c.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	c.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := c.Run()

	exitCode := 0
	startupErrMsg := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			startupErrMsg = fmt.Sprintf("failed to start command: %v", err)
			stderrBuf.WriteString(startupErrMsg)
			// Always print startup error to stderr so the user sees it.
			fmt.Fprintln(os.Stderr, startupErrMsg)
		}
	}

	// Save raw output (ANSI stripped) to log file.
	logPath := saveOutput(command, commandArgs, exitCode, stdoutBuf.Bytes(), stderrBuf.Bytes(), verbose)

	// If command failed, generate a horus error report and append it to the log.
	if err != nil {
		const maxErrLen = 1024
		stderrSample := stderrBuf.String()
		if len(stderrSample) > maxErrLen {
			stderrSample = stderrSample[:maxErrLen] + "... (truncated)"
		}
		wrappedErr := horus.PropagateErr(
			"run command",
			"command_execution_error",
			fmt.Sprintf("command %q failed with exit code %d", command, exitCode),
			err,
			map[string]any{
				"command": command,
				"args":    commandArgs,
				"exit":    exitCode,
				"stderr":  stderrSample,
			},
		)

		var report string
		if herr, ok := wrappedErr.(*horus.Herror); ok {
			report = horus.PseudoJSONFormatter(herr)
			report = stripANSI(report)
		} else {
			report = wrappedErr.Error()
		}

		// Append the report to the log file.
		if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			defer f.Close()
			fmt.Fprintf(f, "\n--- HORUS ERROR REPORT ---\n%s\n", report)
		} else if verbose {
			fmt.Fprintf(os.Stderr, "Warning: could not append error report to log: %v\n", err)
		}
	}

	// Clipboard handling: copy raw stderr/stdout (ANSI stripped) as requested.
	if copyOut {
		stripped := stripANSI(stdoutBuf.String())
		copyToClipboard([]byte(stripped), verbose)
	}
	stderrCopied := false
	if copyErr {
		stripped := stripANSI(stderrBuf.String())
		copyToClipboard([]byte(stripped), verbose)
		stderrCopied = true
	}
	if exitCode != 0 && !stderrCopied {
		// On failure, automatically copy raw stderr (unless already copied by -e).
		stripped := stripANSI(stderrBuf.String())
		copyToClipboard([]byte(stripped), verbose)
	}

	// Exit with the command's exit code (or 1 for startup failure).
	os.Exit(exitCode)
}

// saveOutput writes the captured output to a file in ~/.kage/logs/ and returns the full path.
func saveOutput(command string, args []string, exitCode int, stdout, stderr []byte, verbose bool) string {
	home, err := domovoi.FindHome(verbose)
	if err != nil {
		horus.CheckErr(err, horus.WithOp("save output"), horus.WithMessage("failed to find home directory"))
	}

	logDir := filepath.Join(home, ".kage", "logs")
	if err := domovoi.CreateDir(logDir, verbose); err != nil {
		horus.CheckErr(err, horus.WithOp("save output"), horus.WithMessage("failed to create log directory"))
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	baseCmd := filepath.Base(command)
	filename := fmt.Sprintf("%s_%s.log", timestamp, baseCmd)
	fullPath := filepath.Join(logDir, filename)

	file, err := os.Create(fullPath)
	if err != nil {
		horus.CheckErr(err, horus.WithOp("save output"), horus.WithMessage("failed to create log file"),
			horus.WithDetails(map[string]any{"path": fullPath}))
	}
	defer file.Close()

	fmt.Fprintf(file, "Command: %s %v\n", command, args)
	fmt.Fprintf(file, "Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "Exit Code: %d\n", exitCode)
	fmt.Fprintf(file, "\n--- STDOUT ---\n")
	file.Write([]byte(stripANSI(string(stdout))))
	fmt.Fprintf(file, "\n--- STDERR ---\n")
	file.Write([]byte(stripANSI(string(stderr))))

	if verbose {
		fmt.Printf("Output saved to %s\n", fullPath)
	}
	return fullPath
}

////////////////////////////////////////////////////////////////////////////////////////////////////
