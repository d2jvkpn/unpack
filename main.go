package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func run(args []string, stdout io.Writer, stderr io.Writer, workingDir string) int {
	var (
		flags         *flag.FlagSet
		chinese       *bool
		outputDir     *string
		extractionDir string
		failed        bool
	)

	flags = flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	chinese = flags.Bool("cn", false, "decode legacy Chinese filenames as GBK")
	outputDir = flags.String("output-dir", "", "extract into DIR")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: unpack [--cn] [--output-dir DIR] ARCHIVE...")
		fmt.Fprintln(stderr, "  --cn                 decode legacy Chinese filenames as GBK")
		fmt.Fprintln(stderr, "  --output-dir DIR     extract into DIR")
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
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	extractionDir = *outputDir
	if extractionDir != "" && !filepath.IsAbs(extractionDir) {
		extractionDir = filepath.Join(workingDir, extractionDir)
	}

	for _, archivePath := range flags.Args() {
		if err := processArchive(archivePath, extractionDir, workingDir, *chinese, stdout); err != nil {
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
	stdout io.Writer,
) error {
	var (
		format  archiveFormat
		entries []archiveEntry
		plans   []plannedEntry
		skipped []string
		err     error
	)

	format, err = detectFormat(archivePath)
	if err != nil {
		return fmt.Errorf("process archive %q: %w", archivePath, err)
	}
	entries, err = scanArchive(archivePath, format, chinese)
	if err != nil {
		return err
	}
	plans, _, err = planEntries(entries, archivePath, outputDir, workingDir, format)
	if err != nil {
		return fmt.Errorf("plan archive %q: %w", archivePath, err)
	}

	fmt.Fprintf(stdout, "Extracting: %s\n", archivePath)
	skipped, err = extractArchive(archivePath, format, plans)
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

func main() {
	var (
		workingDir string
		err        error
	)

	workingDir, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: get current working directory: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, workingDir))
}
