# unpack

Extract ZIP, TAR, TAR.GZ, and TGZ archives with safe paths and optional legacy Chinese filenames.

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
unpack [--cn] [--output-dir DIR] ARCHIVE...
```

One or more archive paths are required. Supported filename extensions are `.zip`, `.tar`,
`.tar.gz`, and `.tgz`, matched case-insensitively. Multiple archives are processed in order; a
failure in one archive does not prevent later archives from being processed. The final status is
zero when all archives succeed, one when any archive fails processing, and two for invalid command
usage.

`--output-dir` has no short `-o` alias.

## Output rules

Without `--output-dir`, an archive containing one top-level item extracts directly into the current
directory and retains that item's path. An archive containing multiple top-level items extracts
into a directory named after the archive with its complete recognized suffix removed. For example,
`a.tar.gz` extracts into `./a/`, not `./a.tar/`.

With `--output-dir`, all archives in the invocation share the specified directory. When an archive
contains one top-level directory, that wrapper directory is stripped: `wrapper/note.txt` becomes
`DIR/note.txt`. A sole top-level file is not stripped, and multiple top-level items retain their
paths.

Existing files are never overwritten. Each preserved file is reported as
`Skipping existing file: NAME`, and extraction continues.

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

An explicit output directory:

```sh
unpack --output-dir restored photos.zip documents.tar.gz
```

Both flags together:

```sh
unpack --cn --output-dir restored old-photos.zip
```
