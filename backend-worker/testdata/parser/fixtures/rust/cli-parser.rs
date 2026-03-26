//! CLI argument parser for the file-indexing service.

use std::env;
use std::fs;
use std::io::{self, BufRead, Write};
use std::path::PathBuf;
use std::process;

use clap::{Arg, ArgMatches, Command};
use serde::{Deserialize, Serialize};

/// Supported output formats for index results.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum OutputFormat {
    Json,
    Table,
    Csv,
}

/// Configuration parsed from CLI arguments.
#[derive(Debug, Clone)]
pub struct Config {
    pub root_dir: PathBuf,
    pub output_format: OutputFormat,
    pub max_depth: usize,
    pub include_hidden: bool,
    pub extensions: Vec<String>,
}

/// Trait for components that produce index output.
pub trait Formatter {
    /// Format the given entries and write them to the output.
    fn format(&self, entries: &[IndexEntry], out: &mut dyn Write) -> io::Result<()>;
}

/// A single entry in the file index.
#[derive(Debug, Serialize, Deserialize)]
pub struct IndexEntry {
    pub path: PathBuf,
    pub size_bytes: u64,
    pub line_count: usize,
}

impl IndexEntry {
    /// Create a new IndexEntry by reading file metadata.
    pub fn from_path(path: PathBuf) -> io::Result<Self> {
        let metadata = fs::metadata(&path)?;
        let line_count = count_lines(&path)?;
        Ok(Self {
            path,
            size_bytes: metadata.len(),
            line_count,
        })
    }
}

impl Config {
    /// Build a Config from parsed CLI argument matches.
    pub fn from_matches(matches: &ArgMatches) -> Self {
        let root_dir = PathBuf::from(
            matches
                .get_one::<String>("dir")
                .unwrap_or(&".".to_string()),
        );
        let output_format = match matches.get_one::<String>("format").map(|s| s.as_str()) {
            Some("json") => OutputFormat::Json,
            Some("csv") => OutputFormat::Csv,
            _ => OutputFormat::Table,
        };
        let max_depth = matches
            .get_one::<String>("depth")
            .and_then(|d| d.parse().ok())
            .unwrap_or(10);
        let include_hidden = matches.get_flag("hidden");
        let extensions: Vec<String> = matches
            .get_many::<String>("ext")
            .map(|vals| vals.cloned().collect())
            .unwrap_or_default();

        Config {
            root_dir,
            output_format,
            max_depth,
            include_hidden,
            extensions,
        }
    }
}

/// Build the clap CLI command definition.
pub fn build_cli() -> Command {
    Command::new("file-indexer")
        .version(env!("CARGO_PKG_VERSION"))
        .about("Index source files for analysis")
        .arg(
            Arg::new("dir")
                .short('d')
                .long("dir")
                .help("Root directory to index"),
        )
        .arg(
            Arg::new("format")
                .short('f')
                .long("format")
                .help("Output format: json, table, csv"),
        )
        .arg(
            Arg::new("depth")
                .long("depth")
                .help("Maximum directory depth"),
        )
        .arg(
            Arg::new("hidden")
                .long("hidden")
                .action(clap::ArgAction::SetTrue)
                .help("Include hidden files"),
        )
        .arg(
            Arg::new("ext")
                .short('e')
                .long("ext")
                .action(clap::ArgAction::Append)
                .help("File extensions to include"),
        )
}

/// Count lines in a file using buffered reading.
fn count_lines(path: &PathBuf) -> io::Result<usize> {
    let file = fs::File::open(path)?;
    let reader = io::BufReader::new(file);
    Ok(reader.lines().count())
}

/// Entry point: parse args, build config, and run the indexer.
pub fn run() -> Result<(), Box<dyn std::error::Error>> {
    let matches = build_cli().get_matches();
    let config = Config::from_matches(&matches);

    if !config.root_dir.exists() {
        eprintln!("Error: directory {:?} does not exist", config.root_dir);
        process::exit(1);
    }

    println!("Indexing {:?} (depth={})", config.root_dir, config.max_depth);
    Ok(())
}
