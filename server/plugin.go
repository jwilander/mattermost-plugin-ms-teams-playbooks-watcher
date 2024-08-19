package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/mux"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

const BOT_TOKEN_KEY_PREFIX = "bot-token-key-"

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
	client       *pluginapi.Client
	mmRESTClient *model.Client4

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	router   *mux.Router
	botID    string
	botToken string
}

func (p *Plugin) OnActivate() error {
	return p.initializeAPI()
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) initializeAPI() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    "playbook-watcher",
		DisplayName: "Playbook Watcher",
		Description: "",
	})
	if err != nil {
		return err
	}

	p.botID = botID

	err = p.client.KV.Get(BOT_TOKEN_KEY_PREFIX+botID, &p.botToken)
	if err != nil {
		return err
	}

	if p.botToken == "" {
		token, err := p.client.User.CreateAccessToken(botID, "")
		if err != nil {
			return err
		}
		p.botToken = token.Token
		p.client.KV.Set(BOT_TOKEN_KEY_PREFIX+botID, p.botToken)
	}

	mmConfig := p.client.Configuration.GetConfig()
	p.mmRESTClient = model.NewAPIv4Client(*mmConfig.ServiceSettings.SiteURL)
	p.mmRESTClient.APIURL = *mmConfig.ServiceSettings.SiteURL + "/plugins/playbooks/api/v0"
	p.mmRESTClient.SetToken(p.botToken)

	router := mux.NewRouter()
	router.HandleFunc("/hello", p.handleHello)
	router.HandleFunc("/runs", p.handleRuns)

	p.router = router

	return nil
}

func (p *Plugin) getPlaybookRuns() ([]*PlaybookRun, error) {
	resp, err := p.mmRESTClient.DoAPIGet(context.Background(), "/runs", "")
	if err != nil {
		p.client.Log.Error(err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	var runList PlaybookRunList
	err = json.NewDecoder(resp.Body).Decode(&runList)
	if err != nil {
		p.client.Log.Error(err.Error())
		return nil, err
	}

	return runList.Items, nil
}

func (p *Plugin) handleHello(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("Hello World!")); err != nil {
		p.API.LogError("Failed to write hello world", "err", err.Error())
	}
}

func (p *Plugin) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := p.getPlaybookRuns()
	if err != nil {
		p.API.LogError("Failed to get playbook runs", "err", err.Error())
	}

	responseJSON, _ := json.Marshal(runs)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(responseJSON); err != nil {
		p.API.LogError("Failed to write playbook runs", "err", err.Error())
	}
}
