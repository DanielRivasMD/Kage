////////////////////////////////////////////////////////////////////////////////////////////////////

pub mod completion {

    use anyhow::Result as anyResult;
    use clap::{Command, CommandFactory, arg};
    use clap_complete::{generate, shells::*};
    use std::io;

    use crate::cli;

    pub fn run(shell: cli::Shell) -> anyResult<()> {
        let visible: Vec<_> = cli::Cli::command()
            .get_subcommands()
            .filter(|s| !s.is_hide_set())
            .cloned()
            .collect();

        let mut cmd = Command::new(env!("CARGO_BIN_NAME"))
            .subcommands(visible)
            .arg(arg!(-v --verbose "Enable verbose diagnostics"))
            .arg(arg!(-o --out      "Copy stdout to clipboard"))
            .arg(arg!(-e --err      "Copy stderr to clipboard"));

        let name = cmd.get_name().to_string();

        match shell {
            cli::Shell::Bash => generate(Bash, &mut cmd, name, &mut io::stdout()),
            cli::Shell::Zsh => generate(Zsh, &mut cmd, name, &mut io::stdout()),
            cli::Shell::Fish => generate(Fish, &mut cmd, name, &mut io::stdout()),
            cli::Shell::PowerShell => generate(PowerShell, &mut cmd, name, &mut io::stdout()),
        }
        Ok(())
    }
}

////////////////////////////////////////////////////////////////////////////////////////////////////

pub mod identity {
    use anyhow::Result as anyResult;

    const IDENTITY: &str = r#""#;

    pub fn run() -> anyResult<()> {
        println!("{}", IDENTITY);
        Ok(())
    }
}

////////////////////////////////////////////////////////////////////////////////////////////////////
