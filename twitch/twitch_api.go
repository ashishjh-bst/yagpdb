package twitch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

var (
	twitchClientID     = os.Getenv("YAGPDB_TWITCH_CLIENT_ID")
	twitchClientSecret = os.Getenv("YAGPDB_TWITCH_CLIENT_SECRET")

	tokenLock sync.Mutex
	appToken  string
	tokenExp  time.Time
)

func getAppToken() (string, error) {
	tokenLock.Lock()
	defer tokenLock.Unlock()
	if appToken != "" && time.Now().Before(tokenExp) {
		return appToken, nil
	}
	resp, err := http.PostForm("https://id.twitch.tv/oauth2/token", url.Values{
		"client_id":     {twitchClientID},
		"client_secret": {twitchClientSecret},
		"grant_type":    {"client_credentials"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	appToken = data.AccessToken
	tokenExp = time.Now().Add(time.Duration(data.ExpiresIn-60) * time.Second)
	return appToken, nil
}

func CheckChannelLive(channelName string) (bool, string, error) {
	token, err := getAppToken()
	if err != nil {
		return false, "", err
	}
	url := fmt.Sprintf("https://api.twitch.tv/helix/streams?user_login=%s", channelName)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Client-ID", twitchClientID)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	var data struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, "", err
	}
	if len(data.Data) > 0 {
		return true, data.Data[0].Title, nil
	}
	return false, "", nil
}
