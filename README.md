# unpack

Extract ZIP, TAR, TAR.GZ, and TGZ archives with safe paths, a smart default destination, an
overridable output directory (`--output-dir`), optional extraction of specific files, optional
overwrite of existing files, and optional legacy Chinese filename decoding.

<https://github.com/d2jvkpn/unpack>

## Build

Go 1.27 or newer is required.

```sh
go build -o unpack .
```

## Installation

Download and install a prebuilt binary for Linux (amd64/arm64) or macOS (arm64):

```sh
curl -fsSL https://github.com/d2jvkpn/unpack/releases/latest/download/install.sh | sh
```

The script downloads the matching platform `.zip` release asset, so `unzip` must be available. It
installs into `/usr/local/bin` by default, falling back to `$HOME/.local/bin` if that is not
writable. Set `INSTALL_DIR` to choose a different location, or `VERSION` to install a specific
release tag instead of the latest one.

Alternatively, install from a source checkout into your configured Go binary directory:

```sh
go install .
```

You can also copy the binary produced by the build command to a directory on your `PATH`.

## Usage

```text
unpack [--cn] [--output-dir DIR] [--overwrite] [--version] ARCHIVE [FILE...]
```

Exactly one archive is required, given as a local path or an `http://`/`https://` URL. A URL is
downloaded to a temporary file (following up to 5 redirects, with a 10-second response-header
timeout and a 5-minute overall download timeout) before the usual extraction logic runs; the
temporary file and its containing directory are removed afterward. Supported filename extensions
are `.zip`, `.tar`, `.tar.gz`, and `.tgz`, matched case-insensitively against the filename taken
from the input URL (redirects are followed for the download itself but do not change the filename
used). Any arguments after the archive are file selectors (see
[Selecting specific files](#selecting-specific-files)); with none, the whole archive is extracted.
The final status is zero on success, one when the archive fails to process (including a selector
matching nothing or a download failure), and two for invalid command usage.

`--output-dir` has no short `-o` alias. `--overwrite` and `--version` have no short aliases either.

Before extracting each archive, `unpack` prints the archive path and the resolved output
directory, for example:

```text
Extracting: photos.zip
Output directory: /home/user/photos
```

## Version information

`unpack --version` prints the version, commit, commit time, dirty flag, and build time, and exits
without extracting anything:

```text
version:     0.3.0
commit:      189da1ea953c4cd8381b044057fc7a2f32811113
commit_time: 2026-08-23T14:30:23Z
modified:    false
build_time:  2026-08-23T14:40:49Z
```

`commit`, `commit_time`, and `modified` come from the VCS metadata that `go build` embeds
automatically when built inside a git checkout (no `-ldflags` needed); they report `none`,
`unknown`, and `false` respectively when that metadata isn't available. `build_time` is only
populated when the binary is built with `make build` (or one of the other `make build-*` targets),
which injects it via `-ldflags`. A plain `go build` invocation without that ldflag reports
`build_time: unknown`.

## Output rules

Without `--output-dir`, an archive containing one top-level item extracts directly into the current
directory and retains that item's path. An archive containing multiple top-level items extracts
into a directory named after the archive with its complete recognized suffix removed. For example,
`a.tar.gz` extracts into `./a/`, not `./a.tar/`.

With `--output-dir`, the archive extracts into the specified directory. When an archive contains
one top-level directory, that wrapper directory is stripped: `wrapper/note.txt` becomes
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

## Selecting specific files

Arguments after `ARCHIVE` restrict extraction to entries matching those selectors instead of
extracting everything. Each selector is one of:

- An exact archive-relative path, e.g. `src/main.go`.
- A directory prefix, e.g. `docs`, which also matches everything beneath it (`docs/a.txt`,
  `docs/sub/b.txt`, ...).
- A glob using `*`, `?`, and `[...]`. Unlike `path.Match`, `*` and `?` cross the `/` separator —
  matching the default wildcard behavior of `unzip` and (with `--wildcards`) GNU `tar` — so `*.md`
  matches `README.md` as well as `docs/sub/guide.md`.

When an archive has exactly one top-level directory, selectors match paths relative to that
directory rather than the raw archive path, so `unpack bundle.zip src/main.go` extracts
`bundle-1.0/src/main.go` without needing to name the `bundle-1.0/` wrapper. Archives with multiple
top-level entries match the raw archive path instead.

If any selector matches nothing in the archive, `unpack` reports an error and extracts nothing
(the same scan-before-write safety model applies: no payload is written until every selector is
known to match).

```sh
unpack bundle.zip src/main.go docs '*.md'
```

Quote glob selectors so the shell does not expand them against your local filesystem first.

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
unpack --output-dir restored photos.zip
```

Extract only specific files from an archive:

```sh
unpack --output-dir restored photos.zip vacation/beach.jpg vacation/sunset.jpg
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

Extract an archive directly from a URL:

```sh
unpack https://example.com/releases/photos.zip
```
