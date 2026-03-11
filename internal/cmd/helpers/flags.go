package helpers

import "strings"

// IntersperseFlags moves all flags (and their values) before the positional
// arguments so that kubectl-style flag placement is tolerated.
func IntersperseFlags(args []string) []string {
	var flags []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}

	return append(flags, positional...)
}
