////////////////////////////////////////////////////////////////////////////////////////////////////

use clap::{CommandFactory, Parser};
use clap_complete::generate;
use std::io::{self};

////////////////////////////////////////////////////////////////////////////////////////////////////

mod cli;
mod util;

////////////////////////////////////////////////////////////////////////////////////////////////////

fn main() {
    let cli = cli::Cli::parse();

    // If a subcommand was given, handle it and exit.
    if let Some(cli::Command::Completion { shell }) = cli.cmd {
        let mut cmd = cli::Cli::command();
        let bin_name = cmd.get_name().to_string();
        generate(shell, &mut cmd, bin_name, &mut io::stdout());
        return;
    }

    // clap ensures `command` is non‑empty here because of `required_unless_present`,
    // but we keep the check as a safeguard.
    if cli.command.is_empty() {
        eprintln!("Error: no command provided after --");
        std::process::exit(1);
    }

    let cmd_name = &cli.command[0];
    let cmd_args = &cli.command[1..];

    // Execute the external command, streaming output live
    let (stdout_bytes, stderr_bytes, exit_status) = util::execute_and_capture(cmd_name, cmd_args);

    let exit_code = match exit_status.code() {
        Some(code) => code,
        None => {
            {
                eprintln!("Command terminated abnormally");
                1
            }
        }
    };

    // Strip ANSI sequences from the captured bytes
    let stripped_stdout = util::strip_ansi_bytes(&stdout_bytes);
    let stripped_stderr = util::strip_ansi_bytes(&stderr_bytes);
    let stdout_text = String::from_utf8_lossy(&stripped_stdout);
    let stderr_text = String::from_utf8_lossy(&stripped_stderr);

    // Log output
    util::save_output(
        cmd_name,
        cmd_args,
        exit_code,
        &stdout_bytes,
        &stderr_bytes,
        cli.verbose,
    );

    // Clipboard handling
    let mut stderr_copied = false;
    if cli.out {
        util::copy_text(&stdout_text, cli.verbose);
    }
    if cli.err {
        util::copy_text(&stderr_text, cli.verbose);
        stderr_copied = true;
    }
    if exit_code != 0 && !stderr_copied {
        util::copy_text(&stderr_text, cli.verbose);
    }

    std::process::exit(exit_code);
}

////////////////////////////////////////////////////////////////////////////////////////////////////
