package models

type TwitchChannelSubscription struct {
	ID                int64   `boil:"id" json:"id" toml:"id" yaml:"id"`
	GuildID           int64   `boil:"guild_id" json:"guild_id" toml:"guild_id" yaml:"guild_id"`
	ChannelID         int64   `boil:"channel_id" json:"channel_id" toml:"channel_id" yaml:"channel_id"`
	TwitchChannelName string  `boil:"twitch_channel_name" json:"twitch_channel_name" toml:"twitch_channel_name" yaml:"twitch_channel_name"`
	TwitchChannelID   string  `boil:"twitch_channel_id" json:"twitch_channel_id" toml:"twitch_channel_id" yaml:"twitch_channel_id"`
	MentionEveryone   bool    `boil:"mention_everyone" json:"mention_everyone" toml:"mention_everyone" yaml:"mention_everyone"`
	MentionRoles      []int64 `boil:"mention_roles" json:"mention_roles" toml:"mention_roles" yaml:"mention_roles"`
	Enabled           bool    `boil:"enabled" json:"enabled" toml:"enabled" yaml:"enabled"`
	PublishVODs       bool    `boil:"publish_vods" json:"publish_vods" toml:"publish_vods" yaml:"publish_vods"`
}

// TwitchFeedForm is used for web form binding
type TwitchFeedForm struct {
	TwitchChannelName string
	DiscordChannel    int64 `valid:"channel,false"`
	MentionEveryone   bool
	MentionRoles      []int64
	Enabled           bool
	PublishVODs       bool
}

// TwitchAnnouncement is per-guild custom notification
type TwitchAnnouncement struct {
	GuildID                int64
	Message                string
	Enabled                bool
	VODAnnouncement        string
	VODAnnouncementEnabled bool
}

var (
	twitchSubs          []*TwitchChannelSubscription
	twitchAnnouncements = map[int64]*TwitchAnnouncement{}
)

// GetTwitchAnnouncement returns the announcement for a guild
func GetTwitchAnnouncement(guildID int64) *TwitchAnnouncement {
	if a, ok := twitchAnnouncements[guildID]; ok {
		return a
	}
	return &TwitchAnnouncement{
		GuildID:                guildID,
		Message:                "{{.TwitchChannelName}} is now live! {{.URL}}",
		Enabled:                false,
		VODAnnouncement:        "@{{.TwitchChannelName}} streamed {{.DurationAgo}} ago, watch: {{.VODURL}}",
		VODAnnouncementEnabled: false,
	}
}

// SetTwitchAnnouncement sets the announcement for a guild
func SetTwitchAnnouncement(guildID int64, msg string, enabled bool, vodMsg string, vodEnabled bool) {
	twitchAnnouncements[guildID] = &TwitchAnnouncement{
		GuildID:                guildID,
		Message:                msg,
		Enabled:                enabled,
		VODAnnouncement:        vodMsg,
		VODAnnouncementEnabled: vodEnabled,
	}
}

// GetAllEnabledTwitchSubscriptions returns all enabled Twitch subscriptions (demo: in-memory)
func GetAllEnabledTwitchSubscriptions(_ interface{}) ([]*TwitchChannelSubscription, error) {
	var enabled []*TwitchChannelSubscription
	for _, sub := range twitchSubs {
		if sub.Enabled {
			enabled = append(enabled, sub)
		}
	}
	return enabled, nil
}

// TwitchSubsForGuild returns all Twitch subs for a guild
func TwitchSubsForGuild(guildID int64) []*TwitchChannelSubscription {
	var out []*TwitchChannelSubscription
	for _, sub := range twitchSubs {
		if sub.GuildID == guildID {
			out = append(out, sub)
		}
	}
	return out
}

// AddTwitchSub adds a new Twitch subscription
func AddTwitchSub(sub *TwitchChannelSubscription) {
	twitchSubs = append(twitchSubs, sub)
}

// UpdateTwitchSub updates a Twitch subscription by guild and id
func UpdateTwitchSub(guildID int64, id int64, form *TwitchFeedForm) {
	for _, sub := range twitchSubs {
		if sub.GuildID == guildID && sub.ID == id {
			sub.TwitchChannelName = form.TwitchChannelName
			sub.ChannelID = form.DiscordChannel
			sub.MentionEveryone = form.MentionEveryone
			sub.MentionRoles = form.MentionRoles
			sub.Enabled = form.Enabled
			sub.PublishVODs = form.PublishVODs
			return
		}
	}
}

// RemoveTwitchSub removes a Twitch subscription by guild and id
func RemoveTwitchSub(guildID int64, id int64) {
	for i, sub := range twitchSubs {
		if sub.GuildID == guildID && sub.ID == id {
			twitchSubs = append(twitchSubs[:i], twitchSubs[i+1:]...)
			return
		}
	}
}
