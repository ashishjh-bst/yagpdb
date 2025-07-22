package twitch

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/premium"
	"github.com/botlabs-gg/yagpdb/v2/twitch/models"
	"github.com/botlabs-gg/yagpdb/v2/web"
	"goji.io"
	"goji.io/pat"
)

//go:embed assets/twitch.html
var PageHTML string

func (p *Plugin) InitWeb() {
	web.AddHTMLTemplate("twitch/assets/twitch.html", PageHTML)
	web.AddSidebarItem(web.SidebarCategoryFeeds, &web.SidebarItem{
		Name: "Twitch",
		URL:  "twitch",
		Icon: "fab fa-twitch",
	})

	twMux := goji.NewMux()
	web.CPMux.Handle(pat.New("/twitch/*"), twMux)
	web.CPMux.Handle(pat.New("/twitch"), twMux)

	twMux.Use(web.RequireBotMemberMW)
	twMux.Use(web.RequirePermMW(0))

	mainGetHandler := web.ControllerHandler(p.HandleTwitch, "cp_twitch")
	twMux.Handle(pat.Get("/"), mainGetHandler)
	twMux.Handle(pat.Get(""), mainGetHandler)

	addHandler := web.ControllerPostHandler(p.HandleNew, mainGetHandler, models.TwitchFeedForm{})
	twMux.Handle(pat.Post(""), addHandler)
	twMux.Handle(pat.Post("/"), addHandler)
	twMux.Handle(pat.Post("/:item/update"), web.ControllerPostHandler(p.HandleEdit, mainGetHandler, models.TwitchFeedForm{}))
	twMux.Handle(pat.Post("/:item/delete"), web.ControllerPostHandler(p.HandleRemove, mainGetHandler, nil))
	twMux.Handle(pat.Get("/:item/delete"), web.ControllerPostHandler(p.HandleRemove, mainGetHandler, nil))
	twMux.Handle(pat.Post("/announcement"), web.ControllerPostHandler(p.HandleTwitchAnnouncement, mainGetHandler, TwitchAnnouncementForm{}))
}

type TwitchAnnouncementForm struct {
	Message                string `json:"message" valid:"template,5000"`
	Enabled                bool
	VODAnnouncement        string `json:"vod_announcement" valid:"template,5000"`
	VODAnnouncementEnabled bool
}

func (p *Plugin) HandleTwitch(w http.ResponseWriter, r *http.Request) (web.TemplateData, error) {
	ctx := r.Context()
	activeGuild, templateData := web.GetBaseCPContextData(ctx)
	templateData["Subs"] = models.TwitchSubsForGuild(activeGuild.ID)
	templateData["VisibleURL"] = "/manage/" + strconv.FormatInt(activeGuild.ID, 10) + "/twitch"
	templateData["Announcement"] = models.GetTwitchAnnouncement(activeGuild.ID)
	return templateData, nil
}

func (p *Plugin) HandleTwitchAnnouncement(w http.ResponseWriter, r *http.Request) (web.TemplateData, error) {
	ctx := r.Context()
	activeGuild, templateData := web.GetBaseCPContextData(ctx)
	form := ctx.Value(common.ContextKeyParsedForm).(*TwitchAnnouncementForm)
	isPremium := premium.ContextPremium(ctx)
	if !isPremium {
		return templateData.AddAlerts(web.ErrorAlert("Custom announcements are a premium feature.")), nil
	}
	models.SetTwitchAnnouncement(activeGuild.ID, form.Message, form.Enabled, form.VODAnnouncement, form.VODAnnouncementEnabled)
	return templateData, nil
}

func (p *Plugin) HandleNew(w http.ResponseWriter, r *http.Request) (web.TemplateData, error) {
	ctx := r.Context()
	activeGuild, templateData := web.GetBaseCPContextData(ctx)
	data := ctx.Value(common.ContextKeyParsedForm).(*models.TwitchFeedForm)

	isPremium := premium.ContextPremium(ctx)
	feeds := models.TwitchSubsForGuild(activeGuild.ID)
	maxFeeds := 1
	if isPremium {
		maxFeeds = 10
	}
	if len(feeds) >= maxFeeds {
		return templateData.AddAlerts(web.ErrorAlert(
			fmt.Sprintf("You can only have %d Twitch feed(s) on this server. Upgrade to premium for more.", maxFeeds))), nil
	}

	// Add new subscription
	sub := &models.TwitchChannelSubscription{
		ID:                int64(len(feeds) + 1),
		GuildID:           activeGuild.ID,
		ChannelID:         data.DiscordChannel,
		TwitchChannelName: strings.ToLower(data.TwitchChannelName),
		TwitchChannelID:   strings.ToLower(data.TwitchChannelName), // For demo, use name as ID
		MentionEveryone:   data.MentionEveryone,
		MentionRoles:      data.MentionRoles,
		Enabled:           true,
		PublishVODs:       data.PublishVODs,
	}
	models.AddTwitchSub(sub)
	return templateData, nil
}

func (p *Plugin) HandleEdit(w http.ResponseWriter, r *http.Request) (web.TemplateData, error) {
	ctx := r.Context()
	activeGuild, templateData := web.GetBaseCPContextData(ctx)
	data := ctx.Value(common.ContextKeyParsedForm).(*models.TwitchFeedForm)
	itemID, _ := strconv.ParseInt(pat.Param(r, "item"), 10, 64)
	models.UpdateTwitchSub(activeGuild.ID, itemID, data)
	return templateData, nil
}

func (p *Plugin) HandleRemove(w http.ResponseWriter, r *http.Request) (web.TemplateData, error) {
	ctx := r.Context()
	activeGuild, templateData := web.GetBaseCPContextData(ctx)
	itemID, _ := strconv.ParseInt(pat.Param(r, "item"), 10, 64)
	models.RemoveTwitchSub(activeGuild.ID, itemID)
	return templateData, nil
}
