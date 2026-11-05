package coordination

import (
	"errors"
	"fmt"
	"strings"
)

func parseMemoryFacts(markdown string) ([]string, error) {
	var facts []string
	seen := make(map[string]struct{})
	for index, line := range strings.Split(markdown, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line != strings.TrimLeft(line, " \t") {
			return nil, fmt.Errorf("memory facts: nested or indented list at line %d", index+1)
		}
		fact, ok := factBullet(strings.TrimSpace(line))
		if !ok {
			return nil, fmt.Errorf("memory facts: expected a flat list item at line %d", index+1)
		}
		if fact == "" || strings.HasPrefix(fact, "```") || strings.HasPrefix(fact, "~~~") {
			return nil, errors.New("memory facts: empty or fenced list item")
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}
	return facts, nil
}

func factBullet(line string) (string, bool) {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && line[1] == ' ' {
		return strings.TrimSpace(line[2:]), true
	}
	if index := strings.IndexByte(line, '.'); index > 0 && index+1 < len(line) && line[index+1] == ' ' {
		for _, digit := range line[:index] {
			if digit < '0' || digit > '9' {
				return "", false
			}
		}
		return strings.TrimSpace(line[index+2:]), true
	}
	return "", false
}
