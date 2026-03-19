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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/DanielRivasMD/domovoi"
	"github.com/DanielRivasMD/horus"
)

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
