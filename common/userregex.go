package common

import (
	"fmt"
	"regexp"
	"regexp/syntax"
)

// Matching costs roughly the compiled program size times the input length, and counted repeats such as
// .{0,999} make a short pattern compile to a very large program, so user supplied regexes are capped by
// program size rather than pattern length.
const (
	// Triggers run against every message, a 1000 character literal trigger compiles to about 1000.
	MaxTriggerRegexProgramSize = 1500
	// Leaves room for chunking idioms such as .{1,1000}, which compiles to about 2000.
	MaxTemplateRegexProgramSize = 10000
)

// CheckUserRegex returns an error if pattern is invalid or compiles to more than maxProgramSize instructions.
func CheckUserRegex(pattern string, maxProgramSize int) error {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return err
	}

	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return err
	}

	if len(prog.Inst) > maxProgramSize {
		return fmt.Errorf("regex is too complex (compiles to %d instructions, max %d), try smaller repeat counts", len(prog.Inst), maxProgramSize)
	}
	return nil
}

// CompileUserRegex compiles a user supplied regex after checking it with CheckUserRegex.
func CompileUserRegex(pattern string, maxProgramSize int) (*regexp.Regexp, error) {
	if err := CheckUserRegex(pattern, maxProgramSize); err != nil {
		return nil, err
	}
	return regexp.Compile(pattern)
}
