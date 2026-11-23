package run

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrInsufficientCapabilities = errors.New("run: caller capabilities are insufficient")

type InsufficientCapabilitiesError struct {
	Missing Capabilities
}

func (i *InsufficientCapabilitiesError) Error() string {
	return fmt.Sprintf("%s: requires %s", ErrInsufficientCapabilities, i.Missing)
}

func (i *InsufficientCapabilitiesError) Is(target error) bool {
	return target == ErrInsufficientCapabilities
}

type Capabilities struct {
	ChildRuns      bool
	InterruptTypes []InterruptType
}

func (c Capabilities) Clone() Capabilities {
	c.InterruptTypes = slices.Clone(c.InterruptTypes)
	return c
}

func (c Capabilities) Validate() error {
	for index, kind := range c.InterruptTypes {
		if !kind.Valid() {
			return fmt.Errorf("run: run capability interrupt kind[%d] is unknown", index)
		}
		if index > 0 && c.InterruptTypes[index-1] >= kind {
			return errors.New("run: run capability interrupt kinds must be sorted without duplicates")
		}
	}
	return nil
}

func (c Capabilities) Equal(other Capabilities) bool {
	left := c.Normalized()
	right := other.Normalized()
	return left.ChildRuns == right.ChildRuns && slices.Equal(left.InterruptTypes, right.InterruptTypes)
}

func (c Capabilities) Normalized() Capabilities {
	kinds := slices.Clone(c.InterruptTypes)
	slices.Sort(kinds)
	return Capabilities{
		ChildRuns:      c.ChildRuns,
		InterruptTypes: slices.Compact(kinds),
	}
}

func (c Capabilities) IsEmpty() bool {
	return !c.ChildRuns && len(c.InterruptTypes) == 0
}

func (c Capabilities) MissingFrom(caller Capabilities) Capabilities {
	var missing Capabilities
	if c.ChildRuns && !caller.ChildRuns {
		missing.ChildRuns = true
	}
	for _, kind := range c.InterruptTypes {
		if !slices.Contains(caller.InterruptTypes, kind) {
			missing.InterruptTypes = append(missing.InterruptTypes, kind)
		}
	}
	return missing
}

func (c Capabilities) String() string {
	parts := make([]string, 0, 1+len(c.InterruptTypes))
	if c.ChildRuns {
		parts = append(parts, "child runs")
	}
	for _, kind := range c.InterruptTypes {
		parts = append(parts, string(kind)+" interrupts")
	}
	if len(parts) == 0 {
		return "no optional capabilities"
	}
	return strings.Join(parts, ", ")
}
