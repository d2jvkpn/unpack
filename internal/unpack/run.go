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
		archivePath   string
		selectors     []string
	)

	flags = flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&chinese, "cn", false, "decode legacy Chinese filenames as GBK")
	flags.StringVar(&outputDir, "directory", "", "extract into DIR")
	flags.StringVar(&outputDir, "d", "", "extract into DIR (shorthand for --directory)")
	flags.BoolVar(&overwrite, "overwrite", false, "overwrite existing files and directories")
	flags.BoolVar(&showVersion, "version", false, "print version information and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: unpack [--cn] [--directory DIR] [--overwrite] [--version] ARCHIVE [FILE...]")
		fmt.Fprintln(stderr, "  --cn                 decode legacy Chinese filenames as GBK")
		fmt.Fprintln(stderr, "  --directory, -d DIR  extract into DIR")
		fmt.Fprintln(stderr, "  --overwrite          overwrite existing files and directories")
		fmt.Fprintln(stderr, "  --version            print version information and exit")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "  FILE...              extract only entries matching these selectors")
		fmt.Fprintln(stderr, "                       (exact paths, directory prefixes, or globs;")
		fmt.Fprintln(stderr, "                       globs may cross '/', e.g. *.md)")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "  ARCHIVE may be a local path or an http(s):// URL, downloaded to a")
		fmt.Fprintln(stderr, "  temporary file before extraction.")
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
		var (
			revision   string
			commitTime string
			modified   bool
		)
		revision, commitTime, modified = vcsInfo()
		fmt.Fprintf(stdout, "version:     %s\n", version)
		fmt.Fprintf(stdout, "commit:      %s\n", revision)
		fmt.Fprintf(stdout, "commit_time: %s\n", commitTime)
		fmt.Fprintf(stdout, "modified:    %t\n", modified)
		fmt.Fprintf(stdout, "build_time:  %s\n", buildTime)
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

	archivePath = flags.Args()[0]
	if isRemoteURL(archivePath) {
		fmt.Fprintf(stdout, "Downloading: %s\n", archivePath)
		localPath, cleanup, err := downloadArchive(archivePath)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		defer cleanup()
		archivePath = localPath
	}
	selectors = flags.Args()[1:]
	if err := processArchive(archivePath, extractionDir, workingDir, chinese, overwrite, selectors, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
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
	selectors []string,
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
	if len(selectors) > 0 {
		plans, err = filterPlans(plans, selectors)
		if err != nil {
			return fmt.Errorf("select files in archive %q: %w", archivePath, err)
		}
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
		if arg == "--directory" || arg == "-d" {
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
