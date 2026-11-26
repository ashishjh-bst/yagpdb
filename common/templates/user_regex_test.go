package templates

import (
	"strings"
	"testing"

	"github.com/botlabs-gg/yagpdb/v2/common"
)

func TestUserRegexLimits(t *testing.T) {
	accepted := []string{
		`(?i)^!(help|info|rank)\s*(\d+)?`,
		strings.Repeat("a", 1000),
	}
	for _, p := range accepted {
		if err := common.CheckUserRegex(p, common.MaxTriggerRegexProgramSize); err != nil {
			t.Errorf("trigger %q rejected: %v", p, err)
		}
	}

	if err := common.CheckUserRegex(`.{0,999}z`, common.MaxTriggerRegexProgramSize); err == nil {
		t.Error("expected .{0,999}z to be rejected as a trigger")
	}
	if err := common.CheckUserRegex(`(?s).{1,1000}`, common.MaxTemplateRegexProgramSize); err != nil {
		t.Errorf("the .{1,1000} chunking idiom should still work in templates: %v", err)
	}
	if err := common.CheckUserRegex(strings.Repeat(`.{0,999}`, 10), common.MaxTemplateRegexProgramSize); err == nil {
		t.Error("expected a huge template regex to be rejected")
	}
	if err := common.CheckUserRegex(`(unclosed`, common.MaxTriggerRegexProgramSize); err == nil {
		t.Error("expected an invalid regex to be rejected")
	}
}
