package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func decodeArchiveName(raw string, chinese bool, legacy bool) (string, error) {
	if !chinese || !legacy {
		return raw, nil
	}
	decoded, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), raw)
	if err != nil {
		return "", fmt.Errorf("decode GBK filename %q: %w", raw, err)
	}
	if strings.ContainsRune(decoded, '\uFFFD') {
		return "", fmt.Errorf("decode GBK filename %q: invalid byte sequence", raw)
	}
	return decoded, nil
}
