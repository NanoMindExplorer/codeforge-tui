// Package diff produces simple line-based unified diffs between two
// versions of a file's content. It is used by the write_file tool to show
// the Diff pane what an AI-driven edit actually changed.
package diff

import "strings"

type opKind int

const (
    opEqual opKind = iota
    opDelete
    opInsert
)

type op struct {
    kind opKind
    line string
}

// maxComparisonCells bounds the O(n*m) LCS table so pathologically large
// files can't stall the UI. Above this size we fall back to a coarse
// "full rewrite" diff (every old line removed, every new line added).
const maxComparisonCells = 4_000_000

// maxOutputLines caps how many diff lines are rendered, to keep the Diff
// pane readable for very large files.
const maxOutputLines = 400

// Unified returns a simple +/- line diff between oldContent and newContent,
// prefixed with a header naming the file. It is not byte-identical to GNU
// diff output, but accurately reflects added, removed, and unchanged lines.
func Unified(filename, oldContent, newContent string) string {
    if oldContent == newContent {
        return "@@ " + filename + " @@\n(no changes)\n"
    }

    oldLines := splitLines(oldContent)
    newLines := splitLines(newContent)
    ops := lcsDiff(oldLines, newLines)

    var sb strings.Builder
    sb.WriteString("@@ " + filename + " @@\n")

    shown := 0
    for _, o := range ops {
        if shown >= maxOutputLines {
            sb.WriteString("... (diff truncated)\n")
            break
        }
        switch o.kind {
        case opEqual:
            sb.WriteString("  " + o.line + "\n")
        case opDelete:
            sb.WriteString("- " + o.line + "\n")
        case opInsert:
            sb.WriteString("+ " + o.line + "\n")
        }
        shown++
    }
    return sb.String()
}

func splitLines(s string) []string {
    if s == "" {
        return nil
    }
    return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// lcsDiff computes a line-level diff using classic LCS dynamic programming.
func lcsDiff(a, b []string) []op {
    n, m := len(a), len(b)

    if n*m > maxComparisonCells {
        ops := make([]op, 0, n+m)
        for _, l := range a {
            ops = append(ops, op{opDelete, l})
        }
        for _, l := range b {
            ops = append(ops, op{opInsert, l})
        }
        return ops
    }

    dp := make([][]int, n+1)
    for i := range dp {
        dp[i] = make([]int, m+1)
    }
    for i := n - 1; i >= 0; i-- {
        for j := m - 1; j >= 0; j-- {
            if a[i] == b[j] {
                dp[i][j] = dp[i+1][j+1] + 1
            } else if dp[i+1][j] >= dp[i][j+1] {
                dp[i][j] = dp[i+1][j]
            } else {
                dp[i][j] = dp[i][j+1]
            }
        }
    }

    ops := make([]op, 0, n+m)
    i, j := 0, 0
    for i < n && j < m {
        switch {
        case a[i] == b[j]:
            ops = append(ops, op{opEqual, a[i]})
            i++
            j++
        case dp[i+1][j] >= dp[i][j+1]:
            ops = append(ops, op{opDelete, a[i]})
            i++
        default:
            ops = append(ops, op{opInsert, b[j]})
            j++
        }
    }
    for ; i < n; i++ {
        ops = append(ops, op{opDelete, a[i]})
    }
    for ; j < m; j++ {
        ops = append(ops, op{opInsert, b[j]})
    }
    return ops
}
