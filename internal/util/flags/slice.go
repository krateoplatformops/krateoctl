package flags

import (
	"flag"
	"strings"
)

// Slice defines a repeatable string flag and returns a pointer to the collected values.
//
// This is useful for flags that can appear multiple times on the command line.
// The values will be appended in the order they appear.
//
// Example:
//
//	var ports = Slice("port", nil, "Repeatable port definition")
//	flag.Parse()
//	fmt.Println("Ports:", *ports)
//
// Run with:
//
//	./mytool -port 8080 -port 9090
//
// Output:
//
//	Ports: [8080 9090]
func Slice(name string, defaultValue []string, usage string) *StringSlice {
	s := StringSlice(defaultValue)
	flag.Var(&s, name, usage)
	return &s
}

// custom slice type that satisfies flag.Value
type StringSlice []string

func (s *StringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *StringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}
