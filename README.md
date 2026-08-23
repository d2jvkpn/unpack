# unpack

Extract ZIP, TAR, TAR.GZ, and TGZ archives with safe paths, a smart default destination, an
overridable output directory (`--output-dir`), optional overwrite of existing files, and optional
legacy Chinese filename decoding.

<https://github.com/d2jvkpn/unpack>

## Build

Go 1.27 or newer is required.

```sh
go build -o unpack .
```

## Installation

Install from a source checkout into your configured Go binary directory:

```sh
go install .
```

Alternatively, copy the binary produced by the build command to a directory on your `PATH`.

## Usage

```text
unpack [--cn] [--output-dir DIR] [--overwrite] [--version] ARCHIVE...
```

One or more archive paths are required. Supported filename extensions are `.zip`, `.tar`,
`.tar.gz`, and `.tgz`, matched case-insensitively. Multiple archives are processed in order; a
failure in one archive does not prevent later archives from being processed. The final status is
zero when all archives succeed, one when any archive fails processing, and two for invalid command
usage.

`--output-dir` has no short `-o` alias. `--overwrite` and `--version` have no short aliases either.

Before extracting each archive, `unpack` prints the archive path and the resolved output
directory, for example:

```text
Extracting: photos.zip
Output directory: /home/user/photos
```

## Version information

`unpack --version` prints the version, commit, and build time, and exits without extracting
anything:

```text
version:    0.1.0
commit:     a0fbda3
build_time: 2026-08-23T09:36:37Z
```

`commit` and `build_time` are only populated when the binary is built with `make build` (or one of
the other `make build-*` targets), which injects them via `-ldflags`. A `go build` invocation
without those ldflags reports `commit: none` and `build_time: unknown`.

## Output rules

Without `--output-dir`, an archive containing one top-level item extracts directly into the current
directory and retains that item's path. An archive containing multiple top-level items extracts
into a directory named after the archive with its complete recognized suffix removed. For example,
`a.tar.gz` extracts into `./a/`, not `./a.tar/`.

With `--output-dir`, all archives in the invocation share the specified directory. When an archive
contains one top-level directory, that wrapper directory is stripped: `wrapper/note.txt` becomes
`DIR/note.txt`. A sole top-level file is not stripped, and multiple top-level items retain their
paths.

`--output-dir` is worth setting explicitly in two common cases where the smart default is not
enough:

- The archive has several loose top-level items and you don't want them scattered directly into
  the current directory — pass `--output-dir` to collect everything under one directory you name.
- The archive is a release package whose filename encodes OS/arch (e.g.
  `myapp-linux-amd64.tar.gz`), where the smart default would otherwise create a directory named
  after that full filename. Pass `--output-dir myapp` to land the contents in a short, predictable
  directory name instead of one that repeats the platform suffix.

By default, existing files are never overwritten. Each preserved file is reported as
`Skipping existing file: NAME`, and extraction continues. Pass `--overwrite` to replace existing
files and directories at the destination instead of skipping them.

## Legacy Chinese filenames

For ZIP, `--cn` decodes a filename as GBK exactly when that entry's ZIP UTF-8 language-encoding
flag is not set. It does not test whether the filename bytes happen to form valid UTF-8. This is
equivalent to the Python CP437 recovery expression `name.encode("cp437").decode("gb2312")`: Go
exposes the original filename bytes, so it decodes those bytes directly, and GBK includes GB2312.
ZIP names whose UTF-8 marker is set remain unchanged.

For TAR variants, `--cn` decodes only names that are not valid UTF-8 as GBK. Valid UTF-8 TAR names
remain unchanged.

Invalid GBK filenames cause that archive to be rejected before extraction.

## Safety and unsupported entries

Every archive is fully scanned and its destinations are validated before any payload from that
archive is written. Absolute paths, path traversal, and paths that escape through an existing
symbolic-link component are rejected.

Encrypted ZIP files are unsupported. Symbolic links, hard links, devices, FIFOs, sockets, and
other special or unknown entries are rejected; only regular files and directories are extracted.

## Examples

Default destination rules:

```sh
unpack photos.zip
```

Legacy Chinese filename decoding with the default destination:

```sh
unpack --cn old-photos.zip
```

An explicit output directory, collecting loose top-level items instead of scattering them into the
current directory:

```sh
unpack --output-dir restored photos.zip documents.tar.gz
```

A release package whose filename encodes OS/arch, extracted into a short, predictable directory
name instead of one that repeats the platform suffix:

```sh
unpack --output-dir myapp myapp-linux-amd64.tar.gz
```

Both flags together:

```sh
unpack --cn --output-dir restored old-photos.zip
```

Re-extract into a directory that already has files, replacing them:

```sh
unpack --output-dir restored --overwrite photos.zip
```

Print version information:

```sh
unpack --version
```
