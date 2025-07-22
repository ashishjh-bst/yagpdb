package twitch

import (
	"context"
	"sync"
	"time"

	"bytes"
	"text/template"

	"strconv"

	"github.com/mediocregopher/radix/v3"

	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/common/mqueue"
	"github.com/botlabs-gg/yagpdb/v2/twitch/models"
)

const PollInterval = time.Minute * 2

func (p *Plugin) StartFeed() {
	p.Stop = make(chan *sync.WaitGroup)
	go p.runFeedLoop()
}

func (p *Plugin) StopFeed(wg *sync.WaitGroup) {
	if p.Stop != nil {
		p.Stop <- wg
	} else {
		wg.Done()
	}
}

func (p *Plugin) runFeedLoop() {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case wg := <-p.Stop:
			wg.Done()
			return
		case <-ticker.C:
			p.pollFeeds()
		}
	}
}

func (p *Plugin) pollFeeds() {
	ctx := context.Background()
	subs, err := models.GetAllEnabledTwitchSubscriptions(ctx)
	if err != nil {
		return
	}

	for _, sub := range subs {
		isLive, streamTitle, err := CheckChannelLive(sub.TwitchChannelName)
		if err != nil {
			continue
		}

		redisKey := "twitch:last_live_at:" + sub.TwitchChannelID
		var lastLiveAtStr string
		common.RedisPool.Do(radix.Cmd(&lastLiveAtStr, "GET", redisKey))

		if isLive {
			if lastLiveAtStr == "" {
				// Just went live
				common.RedisPool.Do(radix.Cmd(nil, "SET", redisKey, strconv.FormatInt(time.Now().Unix(), 10)))
				msg := p.renderTwitchAnnouncement(sub, streamTitle)
				mqueue.QueueMessage(&mqueue.QueuedElement{
					GuildID:    sub.GuildID,
					ChannelID:  sub.ChannelID,
					Source:     "twitch",
					MessageStr: msg,
					Priority:   2,
				})
			}
		} else {
			if lastLiveAtStr != "" {
				// Just went offline
				if sub.PublishVODs {
					lastLiveAtUnix, _ := strconv.ParseInt(lastLiveAtStr, 10, 64)
					durAgo := time.Since(time.Unix(lastLiveAtUnix, 0)).Round(time.Minute)
					vodMsg := p.renderTwitchVODAnnouncement(sub, durAgo)
					mqueue.QueueMessage(&mqueue.QueuedElement{
						GuildID:    sub.GuildID,
						ChannelID:  sub.ChannelID,
						Source:     "twitch",
						MessageStr: vodMsg,
						Priority:   2,
					})
				}
				common.RedisPool.Do(radix.Cmd(nil, "DEL", redisKey))
			}
		}
	}
}

func (p *Plugin) renderTwitchAnnouncement(sub *models.TwitchChannelSubscription, streamTitle string) string {
	ann := models.GetTwitchAnnouncement(sub.GuildID)
	url := "https://twitch.tv/" + sub.TwitchChannelName
	ctx := map[string]interface{}{
		"TwitchChannelName": sub.TwitchChannelName,
		"TwitchChannelID":   sub.TwitchChannelID,
		"ChannelID":         sub.ChannelID,
		"IsLive":            true,
		"StreamTitle":       streamTitle,
		"URL":               url,
	}
	if ann.Enabled && ann.Message != "" {
		tmpl, err := template.New("twitch_announcement").Parse(ann.Message)
		if err == nil {
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, ctx)
			if err == nil {
				return buf.String()
			}
		}
	}
	return "**" + sub.TwitchChannelName + "** is now live on Twitch! " + streamTitle + "\n" + url
}

func (p *Plugin) renderTwitchVODAnnouncement(sub *models.TwitchChannelSubscription, durAgo time.Duration) string {
	ann := models.GetTwitchAnnouncement(sub.GuildID)
	vodURL := "https://twitch.tv/" + sub.TwitchChannelName + "/videos"
	ctx := map[string]interface{}{
		"TwitchChannelName": sub.TwitchChannelName,
		"TwitchChannelID":   sub.TwitchChannelID,
		"ChannelID":         sub.ChannelID,
		"DurationAgo":       durAgo.String(),
		"VODURL":            vodURL,
	}
	if ann.VODAnnouncementEnabled && ann.VODAnnouncement != "" {
		tmpl, err := template.New("twitch_vod_announcement").Parse(ann.VODAnnouncement)
		if err == nil {
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, ctx)
			if err == nil {
				return buf.String()
			}
		}
	}
	return "@" + sub.TwitchChannelName + " streamed " + durAgo.String() + " ago, watch: " + vodURL
}
