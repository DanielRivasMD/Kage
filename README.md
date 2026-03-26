# kage, operate your cli with a shadow

[![License](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)

## Overview

Minimalist CLI to run commands & capture output, with automatic clipboard
copying on error

`kage` executes any command, displays its output in real time, and saves both
stdout & stderr to timestamped log files. On failure, it automatically copies
the error output to clipboard, making debugging faster. Additional flags allow
explicit copying of stdout or stderr even on success

### Technical Architecture

`kage` is a Go‑based CLI built with **Cobra**. It disables Cobra's default flag
parsing to allow passing flags directly to the sub‑command. The tool:

- Forks the target command using `exec.Command`
- Ties its stdout/stderr to multi‑writers that both display to the terminal and
  capture into buffers
- Strips ANSI color codes from the captured output before writing to logs
- Saves logs in `~/.kage/logs/` with a timestamped filename
- On command failure, generates a detailed `horus` error report, appends it to
  the log file, and copies the **raw stderr** to the clipboard (unless the `-e`
  flag was already used)
- Offers `-o`/`--out` to copy stdout and `-e`/`--err` to copy stderr on demand
- Preserves the exit code of the executed command

### Logic Schematic

    ┌─────────────────┐
    │ kage <command>  │
    └────────┬────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ Manual flag parsing               │
    │ -v, -o, -e, -- are consumed       │
    │ Everything else becomes command   │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ exec.Command(command, args...)    │
    │ MultiWriter to terminal + buffers │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ Command runs, output displayed    │
    │ in real time                      │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ Capture stdout/stderr in buffers  │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ Save buffers (ANSI stripped)      │
    │ to ~/.kage/logs/                  │
    │ as YYYY-MM-DD_HH-MM-SS_<cmd>.log  │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ If command failed:                │
    │ - Generate horus error report     │
    │ - Append report to log file       │
    │ - Copy stderr to clipboard        │
    │   (unless -e already used)        │
    └────────┬──────────────────────────┘
             │
             ▼
    ┌───────────────────────────────────┐
    │ If -o or -e flags given:          │
    │ copy to clipboard                 │
    └───────────────────────────────────┘

### Storage Layout (~/.kage/)

    ~/.kage/
    └─ logs/                # All captured command output
        └─ 2026-03-18_13-02-47_ls.log   # Example log file

Each log file contains:

    Command: ls [-la]
    Time: 2026-03-18T13:02:47+02:00
    Exit Code: 0

    --- STDOUT ---
    (stdout content)

    --- STDERR ---
    (stderr content)

    --- HORUS ERROR REPORT ---   (only on failure)
    (detailed error report)

### Usage Examples

    # Basic command
    kage ls -la

    # With verbose output
    kage -v echo "hello"

    # Copy stdout to clipboard after execution
    kage -o cat file.txt

    # Copy stderr even if command succeeds
    kage -e sh -c "echo 'warning' >&2"

    # On failure, stderr is automatically copied
    kage false

    # Use -- to separate kage flags from command flags
    kage -- grep -r "pattern" .

    # Repeat the last shell command (if using zsh wrapper – see below)

### Zsh Wrapper for Repeating Last Command

To allow `kage` with no arguments to repeat the most recent shell command, add
this function to your `~/.zshrc`:

    # Wrapper for kage – repeats last shell command when called without arguments
    kage() {
      if [[ $# -eq 0 ]]; then
        # Get the last command from history (ignoring leading spaces and the 'kage' itself if present)
        local last_cmd=$(fc -ln -1 | sed -e 's/^[[:space:]]*//' -e 's/^kage[[:space:]]*//')
        if [[ -z "$last_cmd" ]]; then
          echo "Error: no previous command found" >&2
          return 1
        fi
        # Run the real kage with sh -c and the fetched command
        command kage sh -c "$last_cmd"
      else
        # Normal invocation: pass all arguments to the real kage
        command kage "$@"
      fi
    }

## Installation

### Language-Specific

    Go:  go install github.com/DanielRivasMD/Kage@latest

## License

Copyright (c) 2026

See the [LICENSE](LICENSE) file for license details
