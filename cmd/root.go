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
		rootCmd, err = d.MakeCmd("root", runRoot,
			domovoi.WithArgs(cobra.MinimumNArgs(1)),
		)
		horus.CheckErr(err)

		rootCmd.PersistentFlags().BoolVarP(&rootFlags.verbose, "verbose", "v", false, "Enable verbose diagnostics")
		rootCmd.Version = VERSION

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

func runRoot(cmd *cobra.Command, args []string) {
	verbose := rootFlags.verbose

	command := args[0]
	commandArgs := args[1:]

	var stdoutBuf, stderrBuf bytes.Buffer

	c := exec.Command(command, commandArgs...)

	c.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	c.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := c.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	saveOutput(command, commandArgs, exitCode, stdoutBuf.Bytes(), stderrBuf.Bytes(), verbose)

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
		horus.CheckErr(wrappedErr)
	}
}

func saveOutput(command string, args []string, exitCode int, stdout, stderr []byte, verbose bool) {
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
	file.Write(stdout)
	fmt.Fprintf(file, "\n--- STDERR ---\n")
	file.Write(stderr)

	if verbose {
		fmt.Printf("Output saved to %s\n", fullPath)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////
