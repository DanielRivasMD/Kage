////////////////////////////////////////////////////////////////////////////////////////////////////

use anyhow::Result as anyResult;

////////////////////////////////////////////////////////////////////////////////////////////////////

use crate::cli;
use crate::util;

////////////////////////////////////////////////////////////////////////////////////////////////////

pub fn run(cli: cli::Cli) -> anyResult<()> {
    if cli.cmd.is_empty() {
        eprintln!("Error: no command provided after --");
        std::process::exit(1);
    }

    let cmd_name = &cli.cmd[0];
    let cmd_args = &cli.cmd[1..];

    let (stdout_bytes, stderr_bytes, exit_status) = util::execute_and_capture(cmd_name, cmd_args);

    let exit_code = match exit_status.code() {
        Some(code) => code,
        None => {
            #[cfg(unix)]
            {
                use std::os::unix::process::ExitStatusExt;
                let signal = exit_status.signal().unwrap_or(0);
                eprintln!("Command terminated by signal: {signal}");
                signal + 128
            }
            #[cfg(not(unix))]
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
