package twitch

var DBSchemas = []string{`
CREATE TABLE IF NOT EXISTS twitch_channel_subscriptions (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,

	guild_id BIGINT NOT NULL,
	channel_id BIGINT NOT NULL,
	twitch_channel_id TEXT NOT NULL,
	twitch_channel_name TEXT NOT NULL,

	mention_everyone BOOLEAN NOT NULL,
	mention_roles BIGINT[] NOT NULL DEFAULT '{}',
	publish_vods BOOLEAN NOT NULL DEFAULT FALSE,
	enabled BOOLEAN NOT NULL DEFAULT TRUE
);
`, `
CREATE TABLE IF NOT EXISTS twitch_announcements (
	guild_id BIGINT PRIMARY KEY,
	message TEXT NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT FALSE,
	vod_announcement TEXT NOT NULL,
	vod_announcement_enabled BOOLEAN NOT NULL DEFAULT FALSE
);
`}
