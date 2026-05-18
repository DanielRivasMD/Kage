use arboard::Clipboard;
use chrono::Local;
use clap::{CommandFactory, Parser, ValueHint};
use clap_complete::{Shell, generate};
use regex::Regex;
use std::borrow::Cow;
use std::{
    fs,
    io::{self, Read, Write},
    path::PathBuf,
    process::{Command, ExitStatus, Stdio},
    sync::{Arc, Mutex},
    thread,
};

/// Strips ANSI escape sequences from a byte slice, returning new bytes.
fn strip_ansi_bytes(bytes: &[u8]) -> Vec<u8> {
    let text = String::from_utf8_lossy(bytes);
    let re = Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
    let cleaned = re.replace_all(&text, "");
    match cleaned {
        Cow::Borrowed(_) => bytes.to_vec(),
        Cow::Owned(owned) => owned.into_bytes(),
    }
}

#[derive(Parser, Debug)]
#[command(
    name = "kage",
    version = "0.1.0",
    about = "Run a command and capture its output",
    after_long_help = "Examples:\n  kage -- echo hello world\n  kage --verbose -- ls -la\n  kage -- sh -c \"echo 'complex command' && false\""
)]
struct Cli {
    /// Enable verbose diagnostics
    #[arg(short, long)]
    verbose: bool,

    /// Copy stdout to clipboard
    #[arg(short = 'o', long)]
    out: bool,

    /// Copy stderr to clipboard
    #[arg(short = 'e', long)]
    err: bool,

    /// The command to execute and its arguments
    #[arg(
        value_hint = ValueHint::CommandWithArguments,
        trailing_var_arg = true,
        allow_hyphen_values = true,
        num_args = 1..,
        required = true
    )]
    command: Vec<String>,
}

#[derive(Parser, Debug)]
enum SubCommand {
    /// Generate shell completion script
    Completions {
        /// Shell to generate completions for
        shell: Shell,
    },
}

fn main() {
    // Attempt to parse as the completion subcommand first
    if let Ok(sub) = SubCommand::try_parse_from(std::env::args_os().take_while(|a| a != "--")) {
        match sub {
            SubCommand::Completions { shell } => {
                let mut cmd = Cli::command();
                let bin_name = cmd.get_name().to_string();
                generate(shell, &mut cmd, bin_name, &mut io::stdout());
                return;
            }
        }
    }

    let cli = Cli::parse();

    if cli.command.is_empty() {
        eprintln!("Error: no command provided after --");
        std::process::exit(1);
    }

    let cmd_name = &cli.command[0];
    let cmd_args = &cli.command[1..];

    // Execute the external command, streaming output live
    let (stdout_bytes, stderr_bytes, exit_status) = execute_and_capture(cmd_name, cmd_args);

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
    let stripped_stdout = strip_ansi_bytes(&stdout_bytes);
    let stripped_stderr = strip_ansi_bytes(&stderr_bytes);
    // Convert to strings for clipboard
    let stdout_text = String::from_utf8_lossy(&stripped_stdout);
    let stderr_text = String::from_utf8_lossy(&stripped_stderr);

    // Log output (discard returned path; function handles verbose print)
    save_output(
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
        copy_text(&stdout_text, cli.verbose);
    }
    if cli.err {
        copy_text(&stderr_text, cli.verbose);
        stderr_copied = true;
    }
    if exit_code != 0 && !stderr_copied {
        copy_text(&stderr_text, cli.verbose);
    }

    std::process::exit(exit_code);
}

/// Executes the command while streaming stdout/stderr to the terminal and capturing them.
fn execute_and_capture(cmd: &str, args: &[String]) -> (Vec<u8>, Vec<u8>, ExitStatus) {
    let mut child = Command::new(cmd)
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap_or_else(|e| {
            eprintln!("Failed to start command '{}': {}", cmd, e);
            std::process::exit(1);
        });

    let stdout_handle = child.stdout.take().expect("stdout pipe missing");
    let stderr_handle = child.stderr.take().expect("stderr pipe missing");

    let stdout_buf = Arc::new(Mutex::new(Vec::new()));
    let stderr_buf = Arc::new(Mutex::new(Vec::new()));

    let stdout_clone = Arc::clone(&stdout_buf);
    let stderr_clone = Arc::clone(&stderr_buf);

    // Thread for stdout
    let stdout_thread = thread::spawn(move || {
        read_and_broadcast(stdout_handle, io::stdout(), stdout_clone);
    });

    // Thread for stderr
    let stderr_thread = thread::spawn(move || {
        read_and_broadcast(stderr_handle, io::stderr(), stderr_clone);
    });

    let exit_status = child.wait().unwrap_or_else(|e| {
        eprintln!("Error waiting for command: {}", e);
        std::process::exit(1);
    });

    stdout_thread.join().ok();
    stderr_thread.join().ok();

    let stdout = Arc::try_unwrap(stdout_buf)
        .expect("stdout Arc still alive")
        .into_inner()
        .unwrap();
    let stderr = Arc::try_unwrap(stderr_buf)
        .expect("stderr Arc still alive")
        .into_inner()
        .unwrap();

    (stdout, stderr, exit_status)
}

/// Reads from a pipe, writes each chunk to a terminal, and appends it to a shared buffer.
fn read_and_broadcast<R: Read, W: Write>(
    mut reader: R,
    mut writer: W,
    buffer: Arc<Mutex<Vec<u8>>>,
) {
    let mut chunk = [0u8; 4096];
    loop {
        let n = match reader.read(&mut chunk) {
            Ok(0) => break,
            Ok(n) => n,
            Err(e) => {
                eprintln!("Read error: {}", e);
                break;
            }
        };
        let _ = writer.write_all(&chunk[..n]);
        let _ = writer.flush();
        let mut buf = buffer.lock().unwrap();
        buf.extend_from_slice(&chunk[..n]);
    }
}

/// Strips ANSI codes and writes output to a log file in ~/.kage/logs/.
fn save_output(
    cmd: &str,
    args: &[String],
    exit_code: i32,
    stdout: &[u8],
    stderr: &[u8],
    verbose: bool,
) {
    let home = dirs::home_dir().unwrap_or_else(|| {
        eprintln!("Cannot determine home directory");
        std::process::exit(1);
    });
    let log_dir = home.join(".kage").join("logs");
    fs::create_dir_all(&log_dir).unwrap_or_else(|e| {
        eprintln!("Failed to create log directory: {}", e);
        std::process::exit(1);
    });

    let timestamp = Local::now().format("%Y-%m-%d_%H-%M-%S");
    // Fix temporary value issue: store the PathBuf in a variable
    let cmd_path = PathBuf::from(cmd);
    let base_cmd = cmd_path.file_name().and_then(|n| n.to_str()).unwrap_or(cmd);
    let filename = format!("{}_{}.log", timestamp, base_cmd);
    let log_path = log_dir.join(filename);

    // Store lossy strings in variables to extend their lifetimes
    let stdout_lossy = String::from_utf8_lossy(stdout);
    let stderr_lossy = String::from_utf8_lossy(stderr);
    let ansi_re = Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
    let cleaned_stdout = ansi_re.replace_all(&stdout_lossy, "");
    let cleaned_stderr = ansi_re.replace_all(&stderr_lossy, "");

    let mut file = fs::File::create(&log_path).unwrap_or_else(|e| {
        eprintln!("Failed to create log file: {}", e);
        std::process::exit(1);
    });

    writeln!(file, "Command: {} {:?}", cmd, args).ok();
    writeln!(file, "Time: {}", Local::now().format("%Y-%m-%d %H:%M:%S")).ok();
    writeln!(file, "Exit Code: {}", exit_code).ok();
    writeln!(file).ok();
    writeln!(file, "--- STDOUT ---").ok();
    file.write_all(cleaned_stdout.as_bytes()).ok();
    writeln!(file).ok();
    writeln!(file, "--- STDERR ---").ok();
    file.write_all(cleaned_stderr.as_bytes()).ok();

    if verbose {
        eprintln!("Output saved to {}", log_path.display());
    }
}

/// Copies text to the system clipboard. Prints a warning on failure if verbose.
fn copy_text(text: &str, verbose: bool) {
    if text.is_empty() {
        if verbose {
            eprintln!("No data to copy to clipboard");
        }
        return;
    }
    match Clipboard::new() {
        Ok(mut clipboard) => {
            if let Err(e) = clipboard.set_text(text) {
                if verbose {
                    eprintln!("Warning: failed to set clipboard: {}", e);
                }
            }
        }
        Err(e) => {
            if verbose {
                eprintln!("Warning: clipboard unavailable: {}", e);
            }
        }
    }
}
