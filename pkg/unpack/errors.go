package unpack

import "errors"

// ErrUnsupportedFormat is returned, wrapped, by DetectFormat and ParseFormat when neither a file's
// content nor its extension identifies a supported archive format.
var ErrUnsupportedFormat = errors.New("unsupported archive format")

// ErrNoMatch is returned, wrapped, by FilterPlans when a selector matches no entries.
var ErrNoMatch = errors.New("selector matched no entries")

// ErrUnsafeEntry is returned, wrapped, by Scan and PlanEntries when an archive entry is rejected
// before anything is written to disk: an entry kind this package can't safely represent, an
// undecodable filename, an absolute or traversing path, or a symlink-escape target.
var ErrUnsafeEntry = errors.New("unsafe archive entry")
