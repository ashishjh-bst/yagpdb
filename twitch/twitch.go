package twitch

import (
	"sync"

	"github.com/botlabs-gg/yagpdb/v2/common"
)

type Plugin struct {
	Stop chan *sync.WaitGroup
}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Twitch",
		SysName:  "twitch",
		Category: common.PluginCategoryFeeds,
	}
}

func RegisterPlugin() {
	p := &Plugin{}
	common.RegisterPlugin(p)
}
