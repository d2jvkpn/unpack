package unpack

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func Run(args []string, stdout io.Writer, stderr io.Writer, workingDir string) int {
	var (
		flags         *flag.FlagSet
		chinese       bool
		outputDir     string
		overwrite     bool
		showVersion   bool
		extractionDir string
		failed        bool
	)

	flags = flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&chinese, "cn", false, "decode legacy Chinese filenames as GBK")
	flags.StringVar(&outputDir, "output-dir", "", "extract into DIR")
	flags.BoolVar(&overwrite, "overwrite", false, "overwrite existing files and directories")
	flags.BoolVar(&showVersion, "version", false, "print version information and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: unpack [--cn] [--output-dir DIR] [--overwrite] [--version] ARCHIVE...")
		fmt.Fprintln(stderr, "  --cn                 decode legacy Chinese filenames as GBK")
		fmt.Fprintln(stderr, "  --output-dir DIR     extract into DIR")
		fmt.Fprintln(stderr, "  --overwrite          overwrite existing files and directories")
		fmt.Fprintln(stderr, "  --version            print version information and exit")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Project: https://github.com/d2jvkpn/unpack")
	}

	if err := validateOptionSyntax(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "version:    %s\n", version)
		fmt.Fprintf(stdout, "commit:     %s\n", commit)
		fmt.Fprintf(stdout, "build_time: %s\n", buildTime)
		return 0
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	extractionDir = outputDir
	if extractionDir != "" && !filepath.IsAbs(extractionDir) {
		extractionDir = filepath.Join(workingDir, extractionDir)
	}

	for _, archivePath := range flags.Args() {
		if err := processArchive(archivePath, extractionDir, workingDir, chinese, overwrite, stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func processArchive(
	archivePath string,
	outputDir string,
	workingDir string,
	chinese bool,
	overwrite bool,
	stdout io.Writer,
) error {
	var (
		format       archiveFormat
		entries      []archiveEntry
		plans        []plannedEntry
		resolvedBase string
		skipped      []string
		err          error
	)

	format, err = detectFormat(archivePath)
	if err != nil {
		return fmt.Errorf("process archive %q: %w", archivePath, err)
	}
	entries, err = scanArchive(archivePath, format, chinese)
	if err != nil {
		return err
	}
	plans, resolvedBase, err = planEntries(entries, archivePath, outputDir, workingDir, format)
	if err != nil {
		return fmt.Errorf("plan archive %q: %w", archivePath, err)
	}

	fmt.Fprintf(stdout, "Extracting: %s\n", archivePath)
	fmt.Fprintf(stdout, "Output directory: %s\n", resolvedBase)
	skipped, err = extractArchive(archivePath, format, plans, overwrite)
	for _, relativeName := range skipped {
		fmt.Fprintf(stdout, "Skipping existing file: %s\n", relativeName)
	}
	if err != nil {
		return err
	}
	return nil
}

func validateOptionSyntax(args []string) error {
	var skipOperand bool

	for _, arg := range args {
		if skipOperand {
			skipOperand = false
			continue
		}
		if arg == "--" {
			return nil
		}
		if arg == "--output-dir" {
			skipOperand = true
			continue
		}
		if arg == "-h" || arg == "--help" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") {
			return nil
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("flag provided but not defined: %s", arg)
		}
	}
	return nil
}
