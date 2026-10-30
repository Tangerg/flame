// Command complexity ratchets cyclomatic complexity in production Go code. A
// function above the threshold must be listed in the baseline at no more than
// its recorded score; a baseline entry that no longer applies must be removed,
// so the baseline only ever shrinks.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const threshold = 20

func main() {
	baseline := flag.String("baseline", "", "baseline file of accepted hotspots")
	write := flag.Bool("write", false, "rewrite the baseline from the current tree")
	flag.Parse()
	if *baseline == "" || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: complexity -baseline FILE [-write] ROOT")
		os.Exit(2)
	}
	if err := check(flag.Arg(0), *baseline, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(root, baselinePath string, write bool) error {
	current, err := measure(root)
	if err != nil {
		return err
	}
	if write {
		return writeBaseline(baselinePath, current)
	}
	accepted, err := readBaseline(baselinePath)
	if err != nil {
		return err
	}
	var problems []string
	for _, key := range slices.Sorted(maps.Keys(current)) {
		score := current[key]
		limit, listed := accepted[key]
		switch {
		case !listed:
			problems = append(problems, fmt.Sprintf("%s has complexity %d (> %d); simplify it", key, score, threshold))
		case score > limit:
			problems = append(problems, fmt.Sprintf("%s grew from %d to %d", key, limit, score))
		case score < limit:
			problems = append(problems, fmt.Sprintf("%s improved from %d to %d; lower its baseline", key, limit, score))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(accepted)) {
		if _, still := current[key]; !still {
			problems = append(problems, fmt.Sprintf("%s is no longer above %d; remove it from the baseline", key, threshold))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("complexity: %d problem(s); rerun with -write only after reducing, never to admit growth:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

func measure(root string) (map[string]int, error) {
	scores := make(map[string]int)
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch name := entry.Name(); {
			case path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata" || name == "vendor" || name == "out"):
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if ast.IsGenerated(file) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if score := cyclomatic(function); score > threshold {
				scores[filepath.ToSlash(relative)+":"+functionName(function)] = score
			}
		}
		return nil
	})
	return scores, err
}

// cyclomatic counts one plus every branch point, the gocyclo rule: if, for,
// range, each non-default case or select clause, and each && or ||.
func cyclomatic(function *ast.FuncDecl) int {
	score := 1
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			score++
		case *ast.CaseClause:
			if node.List != nil {
				score++
			}
		case *ast.CommClause:
			if node.Comm != nil {
				score++
			}
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				score++
			}
		}
		return true
	})
	return score
}

func functionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := function.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if index, ok := receiver.(*ast.IndexExpr); ok {
		receiver = index.X
	}
	if index, ok := receiver.(*ast.IndexListExpr); ok {
		receiver = index.X
	}
	if identifier, ok := receiver.(*ast.Ident); ok {
		return identifier.Name + "." + function.Name.Name
	}
	return function.Name.Name
}

func readBaseline(path string) (map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	accepted := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		score, key, found := strings.Cut(line, " ")
		value, err := strconv.Atoi(score)
		if !found || err != nil {
			return nil, fmt.Errorf("complexity: malformed baseline line %q", line)
		}
		accepted[key] = value
	}
	return accepted, scanner.Err()
}

func writeBaseline(path string, scores map[string]int) error {
	var out strings.Builder
	out.WriteString("# Functions accepted above the cyclomatic threshold, written by tooling/complexity -write.\n")
	out.WriteString("# This list may only shrink: reduce a function, then lower or remove its line.\n")
	for _, key := range slices.Sorted(maps.Keys(scores)) {
		fmt.Fprintf(&out, "%d %s\n", scores[key], key)
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}
