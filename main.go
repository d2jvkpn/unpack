package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"unpack/pkg/unpack"
)

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

func run(args []string, stdout io.Writer, stderr io.Writer, workingDir string) int {
	var (
		flags       *flag.FlagSet
		chinese     bool
		outputDir   string
		overwrite   bool
		formatName  string
		format      unpack.Format
		list        bool
		showVersion bool
		archivePath string
		selectors   []string
		result      unpack.Result
		err         error
	)

	flags = flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&chinese, "cn", false, "decode legacy Chinese filenames as GBK")
	flags.StringVar(&outputDir, "directory", "", "extract into DIR")
	flags.StringVar(&outputDir, "d", "", "extract into DIR (shorthand for --directory)")
	flags.BoolVar(&overwrite, "overwrite", false, "overwrite existing files and directories")
	flags.StringVar(&formatName, "format", "", "force archive format: zip, tar, tar.gz, tar.xz, tar.bz, or bz")
	flags.BoolVar(&list, "list", false, "list archive contents without extracting")
	flags.BoolVar(&showVersion, "version", false, "print version information and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: unpack [--cn] [--directory DIR] [--format FORMAT] [--list] [--overwrite] [--version] ARCHIVE [FILE...]")
		fmt.Fprintln(stderr, "  --cn                 decode legacy Chinese filenames as GBK")
		fmt.Fprintln(stderr, "  --directory, -d DIR  extract into DIR")
		fmt.Fprintln(stderr, "  --format FORMAT      force archive format: zip, tar, tar.gz, tar.xz, tar.bz, or bz")
		fmt.Fprintln(stderr, "                       (default: detect from content, then file suffix)")
		fmt.Fprintln(stderr, "  --list               list first-level archive contents without extracting")
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

	if err = validateOptionSyntax(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err = flags.Parse(args); err != nil {
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
	if formatName != "" {
		format, err = unpack.ParseFormat(formatName)
		if err != nil {
			fmt.Fprintf(stderr, "Error: --format: %v\n", err)
			return 2
		}
	}
	archivePath = flags.Args()[0]
	if unpack.IsRemoteURL(archivePath) {
		fmt.Fprintf(stdout, "Downloading: %s\n", archivePath)
		localPath, cleanup, err := unpack.Download(archivePath)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		defer cleanup()
		archivePath = localPath
	}
	selectors = flags.Args()[1:]
	if list {
		if len(selectors) > 0 {
			fmt.Fprintln(stderr, "Error: --list does not accept file selectors")
			return 2
		}
		if format == 0 {
			format, err = unpack.DetectFormat(archivePath)
			if err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
		}
		var entries []unpack.Entry
		entries, err = listArchiveEntries(archivePath, format, chinese)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		for _, name := range topLevelEntryNames(entries) {
			fmt.Fprintln(stdout, name)
		}
		return 0
	}

	result, err = unpack.Extract(archivePath, unpack.Options{
		Directory:  outputDir,
		WorkingDir: workingDir,
		Chinese:    chinese,
		Overwrite:  overwrite,
		Selectors:  selectors,
		Format:     format,
	})
	if result.Directory != "" {
		fmt.Fprintf(stdout, "Extracting: %s\n", archivePath)
		fmt.Fprintf(stdout, "Output directory: %s\n", result.Directory)
		for _, relativeName := range result.Skipped {
			fmt.Fprintf(stdout, "Skipping existing file: %s\n", relativeName)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func topLevelEntryNames(entries []unpack.Entry) []string {
	var (
		names []string
		seen  map[string]struct{}
	)

	seen = make(map[string]struct{})
	for _, entry := range entries {
		name := strings.TrimPrefix(entry.Name, "./")
		if name == "" || name == "." {
			continue
		}
		first := name
		isDirectory := entry.Kind == unpack.EntryDirectory
		if index := strings.IndexByte(name, '/'); index >= 0 {
			first = name[:index]
			isDirectory = true
		}
		if isDirectory {
			first += "/"
		}
		if _, ok := seen[first]; ok {
			continue
		}
		seen[first] = struct{}{}
		names = append(names, first)
	}
	return names
}

func listArchiveEntries(path string, format unpack.Format, chinese bool) ([]unpack.Entry, error) {
	var (
		tarPath string
		output  []byte
		err     error
	)

	if format == unpack.FormatTarXz && !chinese {
		tarPath, err = exec.LookPath("tar")
		if err == nil {
			output, err = exec.Command(
				tarPath,
				"--list",
				"--xz",
				"--file="+path,
			).CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("list archive %q with tar: %w: %s", path, err, strings.TrimSpace(string(output)))
			}
			return parseTarListOutput(output), nil
		}
	}
	return unpack.Scan(path, format, chinese)
}

func parseTarListOutput(output []byte) []unpack.Entry {
	var entries []unpack.Entry

	for _, name := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		if name == "" {
			continue
		}
		kind := unpack.EntryFile
		if strings.HasSuffix(name, "/") {
			kind = unpack.EntryDirectory
		}
		entries = append(entries, unpack.Entry{Name: name, Kind: kind})
	}
	return entries
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
		if arg == "--directory" || arg == "-d" || arg == "--format" {
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
