////////////////////////////////////////////////////////////////////////////////////////////////////

use clap::{Parser, Subcommand, ValueHint};
use clap_complete::Shell;

////////////////////////////////////////////////////////////////////////////////////////////////////

#[derive(Parser, Debug)]
#[command(
    name = "kage",
    version = "0.1.0",
    about = "Run a command and capture its output",
    after_long_help = "Examples:\n  kage -- echo hello world\n  kage --verbose -- ls -la\n  kage -- sh -c \"echo 'complex command' && false\"",
    subcommand_negates_reqs = true
)]
pub struct Cli {
    /// Enable verbose diagnostics
    #[arg(short, long)]
    pub verbose: bool,

    /// Copy stdout to clipboard
    #[arg(short = 'o', long)]
    pub out: bool,

    /// Copy stderr to clipboard
    #[arg(short = 'e', long)]
    pub err: bool,

    /// Subcommand (e.g., completions)
    #[command(subcommand)]
    pub cmd: Option<Command>,

    /// The command to execute and its arguments
    #[arg(
        value_hint = ValueHint::CommandWithArguments,
        trailing_var_arg = true,
        allow_hyphen_values = true,
        num_args = 1..,
        required = true
    )]
    pub command: Vec<String>,
}

////////////////////////////////////////////////////////////////////////////////////////////////////

#[derive(Debug, Subcommand)]
pub enum Command {
    /// Generate shell completion script
    Completion {
        /// Shell for which to generate completions
        #[arg(value_enum)]
        shell: Shell,
    },
}

////////////////////////////////////////////////////////////////////////////////////////////////////
