package templates

import (
	"encoding/json"
	"testing"

	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
)

func intPtr(i int) *int { return &i }

// buildSampleComponents returns a representative components v2 tree covering
// every top-level component the reverse converter supports.
func buildSampleComponents() []discordgo.TopLevelComponent {
	return []discordgo.TopLevelComponent{
		&discordgo.TextDisplay{Content: "hello world"},
		&discordgo.Separator{Spacing: discordgo.SeparatorSpacingLarge},
		&discordgo.ActionsRow{Components: []discordgo.InteractiveComponent{
			&discordgo.Button{Label: "click", Style: discordgo.SuccessButton, CustomID: "templates-go"},
			&discordgo.Button{Label: "open", Style: discordgo.LinkButton, URL: "https://example.com"},
			&discordgo.Button{Label: "react", Style: discordgo.PrimaryButton, CustomID: "templates-react", Emoji: &discordgo.ComponentEmoji{Name: "wave", ID: 123456789, Animated: true}},
		}},
		&discordgo.ActionsRow{Components: []discordgo.InteractiveComponent{
			&discordgo.SelectMenu{
				MenuType:    discordgo.StringSelectMenu,
				CustomID:    "templates-pick",
				Placeholder: "pick one",
				MinValues:   intPtr(1),
				MaxValues:   2,
				Options: []discordgo.SelectMenuOption{
					{Label: "a", Value: "a", Description: "first", Default: true},
					{Label: "b", Value: "b"},
				},
			},
		}},
		&discordgo.Section{
			Components: []discordgo.SectionComponentPart{
				&discordgo.TextDisplay{Content: "section text"},
			},
			Accessory: &discordgo.Thumbnail{Media: discordgo.UnfurledMediaItem{URL: "https://example.com/thumb.png"}, Description: "thumb"},
		},
		&discordgo.MediaGallery{Items: []discordgo.MediaGalleryItem{
			{Media: discordgo.UnfurledMediaItem{URL: "https://example.com/1.png"}, Description: "one"},
			{Media: discordgo.UnfurledMediaItem{URL: "https://example.com/2.png"}, Spoiler: true},
		}},
		&discordgo.Container{
			AccentColor: 16711680,
			Spoiler:     true,
			Components: []discordgo.TopLevelComponent{
				&discordgo.TextDisplay{Content: "inside container"},
				&discordgo.ActionsRow{Components: []discordgo.InteractiveComponent{
					&discordgo.Button{Label: "nested", Style: discordgo.DangerButton, CustomID: "templates-nested"},
				}},
			},
		},
	}
}

// normalize marshals a component tree to a stable JSON form for comparison. The
// API-provided fields (proxy_url, width, ...) are not part of what we rebuild,
// but our sample data doesn't set them, so a direct marshal round-trips cleanly.
func normalize(t *testing.T, components []discordgo.TopLevelComponent) string {
	t.Helper()
	b, err := json.Marshal(components)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestComponentBuilderFromMessageRoundTrip(t *testing.T) {
	original := buildSampleComponents()
	want := normalize(t, original)

	msg := &discordgo.Message{Components: original}
	cb, err := CreateComponentBuilder(msg)
	if err != nil {
		t.Fatalf("CreateComponentBuilder(message): %v", err)
	}

	rebuilt, err := cb.ToComplexMessage()
	if err != nil {
		t.Fatalf("ToComplexMessage: %v", err)
	}

	got := normalize(t, rebuilt.Components)
	if got != want {
		t.Errorf("round trip mismatch\n want: %s\n  got: %s", want, got)
	}
}

func TestComponentBuilderFromMessageInputs(t *testing.T) {
	// a value message is accepted too, not just a pointer
	msg := discordgo.Message{Components: []discordgo.TopLevelComponent{&discordgo.TextDisplay{Content: "hi"}}}
	cb, err := CreateComponentBuilder(msg)
	if err != nil {
		t.Fatalf("value message input: %v", err)
	}
	if len(cb.Components) != 1 || cb.Components[0] != "text" {
		t.Errorf("unexpected decomposition: %#v", cb.Components)
	}
}
