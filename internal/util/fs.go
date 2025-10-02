package util

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
)

func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func RefToRefName(ref string) plumbing.ReferenceName {
	if ref == "" {
		return ""
	}

	if len(ref) > 0 && (ref[0] == 'v' || ref[0] == 'V') {
		return plumbing.NewTagReferenceName(ref)
	}

	return plumbing.NewBranchReferenceName(ref)
}
