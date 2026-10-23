# Performance

## XZ archive listing

The XZ decoder used by the Go library (`github.com/ulikunitz/xz`) is substantially slower than
the native `xz` implementation on large TAR.XZ archives. This matters even when `unpack --list`
only prints first-level entries: TAR is a sequential format, so the compressed stream must still
be decoded far enough to inspect every TAR header. The implementation also reads the complete
stream so XZ integrity errors are detected.

Reference archive used for measurements:

- `data/node-v24.22.1-linux-x64.tar.xz`
- compressed size: 30.5 MiB
- uncompressed size: 197.5 MiB
- entries: 5,888

On the development Linux environment, the measurements were:

| Command | Elapsed time |
| --- | ---: |
| `tar -tJf data/node-v24.22.1-linux-x64.tar.xz >/dev/null` | about 1.8 s |
| Go XZ scan used by the original `unpack --list` | about 39.7 s |
| optimized `unpack --list data/node-v24.22.1-linux-x64.tar.xz` | about 1.3 s |

The CLI therefore uses the system `tar --list --xz` fast path for TAR.XZ listing when `tar` is
available and legacy Chinese filename decoding is not requested. It invokes `tar` directly with
argument passing, without a shell. If `tar` is unavailable, it falls back to the Go scanner.

## Extraction trade-off

Extraction cannot blindly delegate to system `tar`: the library's safety model requires scanning
and validating every entry before writing payload files. Direct delegation would change the
handling of path traversal, symbolic-link escapes, and unsupported special entries.

The safe extraction pipeline is therefore:

1. scan and validate the complete archive;
2. determine the extraction plan;
3. reopen the stream and write the validated plan.

For TAR.XZ this means the stream is decoded more than once. A possible future optimization is to
use native XZ/TAR tooling to create a temporary uncompressed TAR, validate that temporary stream
with the existing planner, and then write the validated entries. That would trade CPU time for
temporary disk space (roughly the uncompressed archive size) and must preserve cleanup and error
handling guarantees.
