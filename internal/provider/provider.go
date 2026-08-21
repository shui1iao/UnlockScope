// Package provider defines service checks and the built-in provider catalog.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shui1iao/UnlockScope/internal/model"
	"github.com/shui1iao/UnlockScope/internal/probe"
)

// Provider is the extension point for a service check. Implementations should
// avoid credentials and keep all service-specific assumptions in Definition.
type Provider interface {
	Definition() Definition
	Check(context.Context, *probe.Client, string) model.Result
}

// Definition is declarative metadata for a provider.
type Definition struct {
	ID                 string
	Service            string
	Category           string
	Groups             []string
	Regions            []string
	URL                string
	Kind               string // html, json, edit
	CredentialRequired bool
	AvailableWords     []string
	RegionWords        []string
	UnavailableWords   []string
	UnknownWords       []string
	PassStatusOnly     bool
}

type serviceProvider struct{ def Definition }

func (p serviceProvider) Definition() Definition { return p.def }

func (p serviceProvider) Check(ctx context.Context, client *probe.Client, region string) (result model.Result) {
	started := time.Now()
	result = model.Result{
		ID: p.def.ID, Service: p.def.Service, Category: p.def.Category,
		Regions: append([]string{}, p.def.Regions...), Region: region,
		State: model.Unknown, CheckedAt: started,
	}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if p.def.CredentialRequired {
		result.Note = "需要凭据，未发送认证请求"
		return result
	}
	response, err := client.Get(ctx, p.def.URL)
	if err != nil {
		result.State = model.Failed
		if ctx.Err() != nil {
			result.Note = "请求超时或被全局超时取消"
		} else {
			result.Note = "网络请求失败"
		}
		return result
	}
	body := strings.ToLower(string(response.Body))
	validJSON := false
	if p.def.Kind == "json" {
		var value any
		if err := json.Unmarshal(response.Body, &value); err != nil {
			result.State = model.Unknown
			result.Note = "服务响应不是稳定 JSON，无法判断区域"
			return result
		}
		validJSON = true
	}
	if response.StatusCode == 401 || response.StatusCode == 407 {
		result.State, result.Note = model.Unknown, "服务要求登录或认证"
		return result
	}
	if response.StatusCode == 429 {
		result.State, result.Note = model.Unknown, "服务限流，未作确定判断"
		return result
	}
	if response.StatusCode == 403 || response.StatusCode == 451 {
		if containsAny(body, append(append([]string{}, p.def.RegionWords...), "country", "region", "territory", "geoblock")...) {
			result.State, result.Note = model.RegionOnly, "服务返回地区限制信号"
		} else if response.StatusCode == 451 {
			result.State, result.Note = model.RegionOnly, "HTTP 451 表示地区或法律限制"
		} else {
			result.State, result.Note = model.Unknown, "HTTP 403，可能是 WAF、登录墙或地区限制，无法确定"
		}
		return result
	}
	if response.StatusCode >= 500 {
		result.State, result.Note = model.Failed, fmt.Sprintf("服务端 HTTP %d", response.StatusCode)
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		result.State, result.Note = model.Unknown, fmt.Sprintf("HTTP %d，无法可靠判断", response.StatusCode)
		return result
	}
	if containsAny(body, p.def.RegionWords...) {
		result.State, result.Note = model.RegionOnly, "页面包含地区限制信号"
		return result
	}
	if containsAny(body, p.def.UnavailableWords...) {
		result.State, result.Note = model.Unavailable, "页面包含服务不可用信号"
		return result
	}
	if containsAny(body, p.def.UnknownWords...) {
		result.State, result.Note = model.Unknown, "页面要求登录或使用动态脚本，无法确定"
		return result
	}
	// Region is the egress/selected scope supplied by the caller. Provider pages
	// often contain unrelated locale metadata such as region=af; never let
	// untrusted response HTML overwrite the actual egress region.
	if containsAny(body, p.def.AvailableWords...) || p.def.PassStatusOnly || validJSON {
		result.State = model.Available
	} else {
		result.State = model.Unknown
		result.Note = "页面可访问，但没有足够信号确认内容解锁"
		return result
	}
	switch p.def.Kind {
	case "json":
		result.Note = "公开 JSON 接口可访问；未模拟登录或购买"
	case "edit":
		result.Note = "编辑页面可访问；未执行写入"
	default:
		result.Note = "公开页面可访问；结果为启发式判断"
	}
	return result
}

func containsAny(body string, words ...string) bool {
	for _, word := range words {
		if word != "" && strings.Contains(body, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

var commonRegionWords = []string{
	"not available in your country", "not available in your region",
	"not available in this location", "unavailable in your region",
	"not available in this country or region", "content is not available in your region",
	"service is not available in your region", "geoblocked", "geo-blocked", "地域制限",
	"该地区不可用", "此內容不適用", "此内容不可用",
}
var commonUnavailableWords = []string{
	"service unavailable", "temporarily unavailable", "access denied",
	"page not found", "this site can’t be reached", "this site can't be reached",
}
var commonUnknownWords = []string{"sign in to continue", "please log in to continue", "requires javascript"}

func mk(id, service, rawURL, category string, groups, regions []string) Provider {
	markers := []string{service, strings.ReplaceAll(id, "-", " ")}
	if fields := strings.Fields(service); len(fields) > 0 && len(fields[0]) >= 4 {
		markers = append(markers, fields[0])
	}
	return serviceProvider{def: Definition{
		ID: id, Service: service, URL: rawURL, Category: category,
		Groups: groups, Regions: regions, Kind: "html",
		AvailableWords: markers, RegionWords: commonRegionWords, UnavailableWords: commonUnavailableWords,
		UnknownWords: commonUnknownWords,
	}}
}
func mkStatus(id, service, rawURL, category string, groups, regions []string) Provider {
	p := mk(id, service, rawURL, category, groups, regions).(serviceProvider)
	p.def.PassStatusOnly = true
	return p
}
func mkJSON(id, service, rawURL, category string, groups, regions []string) Provider {
	p := mk(id, service, rawURL, category, groups, regions).(serviceProvider)
	p.def.Kind = "json"
	return p
}
func mkEdit(id, service, rawURL, category string, groups, regions []string) Provider {
	p := mk(id, service, rawURL, category, groups, regions).(serviceProvider)
	p.def.Kind = "edit"
	return p
}

// All returns a fresh slice in stable display order. Definitions themselves are
// immutable after construction and contain no credentials or user data.
func All() []Provider {
	globalStreaming := []string{"global"}
	globalAI := []string{"global", "ai"}
	globalSocial := []string{"global", "social"}
	globalSports := []string{"global", "sports"}
	globalGames := []string{"global", "games"}
	globalKnowledge := []string{"global", "knowledge"}
	providers := []Provider{
		mk("tiktok", "TikTok", "https://www.tiktok.com/", "social", globalSocial, nil),
		mk("netflix", "Netflix", "https://www.netflix.com/", "streaming", globalStreaming, nil),
		mk("disney-plus", "Disney+", "https://www.disneyplus.com/", "streaming", globalStreaming, nil),
		mk("youtube-premium", "YouTube Premium", "https://www.youtube.com/premium", "streaming", globalStreaming, nil),
		mkStatus("youtube-cdn", "YouTube CDN", "https://www.youtube.com/generate_204", "streaming", globalStreaming, nil),
		mk("prime-video", "Prime Video", "https://www.primevideo.com/", "streaming", globalStreaming, nil),
		mk("spotify", "Spotify", "https://open.spotify.com/", "streaming", globalStreaming, nil),
		mk("dazn", "DAZN", "https://www.dazn.com/", "sports", globalSports, nil),
		mk("max", "Max", "https://www.max.com/", "streaming", globalStreaming, nil),
		mk("hulu", "Hulu", "https://www.hulu.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("paramount-plus", "Paramount+", "https://www.paramountplus.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("peacock", "Peacock", "https://www.peacocktv.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("crunchyroll", "Crunchyroll", "https://www.crunchyroll.com/", "streaming", globalStreaming, nil),
		mk("apple-tv-plus", "Apple TV+", "https://tv.apple.com/", "streaming", globalStreaming, nil),
		mk("espn-plus", "ESPN+", "https://www.espn.com/watch/", "sports", []string{"na", "sports"}, []string{"na"}),
		mk("pluto-tv", "Pluto TV", "https://pluto.tv/", "streaming", globalStreaming, nil),
		mk("tubi", "Tubi", "https://tubitv.com/", "streaming", globalStreaming, nil),
		mk("iqiyi", "iQIYI", "https://www.iq.com/", "streaming", globalStreaming, nil),
		mk("viki", "Viki", "https://www.viki.com/", "streaming", globalStreaming, nil),
		mk("bilibili", "Bilibili", "https://www.bilibili.com/", "streaming", append(globalStreaming, "tw"), []string{"tw"}),
		mk("soundcloud", "SoundCloud", "https://soundcloud.com/", "streaming", globalStreaming, nil),
		mk("deezer", "Deezer", "https://www.deezer.com/", "streaming", globalStreaming, nil),
		mk("tidal", "TIDAL", "https://tidal.com/", "streaming", globalStreaming, nil),
		mk("twitch", "Twitch", "https://www.twitch.tv/", "social", globalSocial, nil),
		mk("kick", "Kick", "https://kick.com/", "social", globalSocial, nil),
		mk("chatgpt", "ChatGPT", "https://chatgpt.com/", "ai", globalAI, nil),
		mk("claude", "Claude", "https://claude.ai/", "ai", globalAI, nil),
		mk("gemini", "Gemini", "https://gemini.google.com/", "ai", globalAI, nil),
		mk("copilot", "Microsoft Copilot", "https://copilot.microsoft.com/", "ai", globalAI, nil),
		mk("grok", "Grok", "https://grok.com/", "ai", globalAI, nil),
		mk("perplexity", "Perplexity", "https://www.perplexity.ai/", "ai", globalAI, nil),
		mk("meta-ai", "Meta AI", "https://www.meta.ai/", "ai", globalAI, nil),
		mk("poe", "Poe", "https://poe.com/", "ai", globalAI, nil),
		mk("reddit", "Reddit", "https://www.reddit.com/", "social", globalSocial, nil),
		mkEdit("wikipedia-editability", "Wikipedia editability", "https://en.wikipedia.org/w/index.php?title=Main_Page&action=edit", "knowledge", globalKnowledge, nil),
		mkJSON("steam-currency", "Steam store/currency", "https://store.steampowered.com/api/featuredcategories/", "games", globalGames, nil),
		mk("epic-games-store", "Epic Games Store", "https://store.epicgames.com/", "games", globalGames, nil),
		mk("xbox", "Xbox", "https://www.xbox.com/", "games", globalGames, nil),
		mk("playstation-store", "PlayStation Store", "https://store.playstation.com/", "games", globalGames, nil),
		mk("nintendo-eshop", "Nintendo eShop", "https://www.nintendo.com/store/", "games", globalGames, nil),
		mk("roblox", "Roblox", "https://www.roblox.com/", "games", globalGames, nil),
		mk("viu-hk", "Viu Hong Kong", "https://www.viu.com/ott/hk/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("now-tv-hk", "Now TV Hong Kong", "https://now.com.hk/", "streaming", []string{"hk", "sports"}, []string{"hk"}),
		mk("mytv-super-hk", "myTV SUPER", "https://www.mytvsuper.com/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("tvb-hk", "TVB Hong Kong", "https://www.tvb.com/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("kkbox-tw", "KKBOX Taiwan", "https://www.kkbox.com/tw/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("friday-video-tw", "friday影音", "https://video.friday.tw/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("line-tv-tw", "LINE TV Taiwan", "https://www.linetv.tw/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("bahamut-tw", "巴哈姆特動畫瘋", "https://ani.gamer.com.tw/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("abema-jp", "ABEMA", "https://abema.tv/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("u-next-jp", "U-NEXT", "https://video.unext.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("fod-jp", "FOD", "https://fod.fujitv.co.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("niconico-jp", "ニコニコ", "https://www.nicovideo.jp/", "social", []string{"jp"}, []string{"jp"}),
		mk("radiko-jp", "radiko", "https://radiko.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("wavve-kr", "Wavve", "https://www.wavve.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("watcha-kr", "Watcha", "https://watcha.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("tving-kr", "TVING", "https://www.tving.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("coupang-play-kr", "Coupang Play", "https://www.coupangplay.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("melon-kr", "Melon", "https://www.melon.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("rtbf-eu", "RTBF Auvio", "https://auvio.rtbf.be/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("svt-eu", "SVT Play", "https://www.svtplay.se/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("france-tv-eu", "France.tv", "https://www.france.tv/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("joyn-eu", "Joyn", "https://www.joyn.de/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("viaplay-eu", "Viaplay", "https://viaplay.com/", "streaming", []string{"eu", "sports"}, []string{"eu"}),
		mk("globoplay-sa", "Globoplay", "https://globoplay.globo.com/", "streaming", []string{"sa"}, []string{"sa"}),
		mk("vix-sa", "ViX", "https://vix.com/", "streaming", []string{"sa"}, []string{"sa"}),
		mk("claro-video-sa", "Claro video", "https://www.clarovideo.com/", "streaming", []string{"sa"}, []string{"sa"}),
		mk("showmax-af", "Showmax", "https://www.showmax.com/", "streaming", []string{"af"}, []string{"af"}),
		mk("dstv-af", "DStv", "https://www.dstv.com/", "streaming", []string{"af", "sports"}, []string{"af"}),
		mk("stan-oc", "Stan", "https://www.stan.com.au/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("kayo-oc", "Kayo Sports", "https://kayosports.com.au/", "sports", []string{"oc", "sports"}, []string{"oc"}),
		mk("9now-oc", "9Now", "https://www.9now.com.au/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("amc-plus", "AMC+", "https://www.amcplus.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("britbox", "BritBox", "https://www.britbox.com/", "streaming", append(globalStreaming, "eu"), []string{"eu", "na"}),
		mk("discovery-plus", "Discovery+", "https://www.discoveryplus.com/", "streaming", globalStreaming, nil),
		mk("fubo", "Fubo", "https://www.fubo.tv/", "sports", append(globalSports, "na"), []string{"na"}),
		mk("sling", "Sling TV", "https://www.sling.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("starz", "STARZ", "https://www.starz.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("mubi", "MUBI", "https://mubi.com/", "streaming", globalStreaming, nil),
		mk("plex", "Plex", "https://watch.plex.tv/", "streaming", globalStreaming, nil),
		mk("rakuten-tv", "Rakuten TV", "https://www.rakuten.tv/", "streaming", globalStreaming, nil),
		mk("curiosity-stream", "Curiosity Stream", "https://curiositystream.com/", "streaming", globalStreaming, nil),
		mk("nebula", "Nebula", "https://nebula.tv/", "streaming", globalStreaming, nil),
		mk("youtube-tv", "YouTube TV", "https://tv.youtube.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("deezer-music", "Deezer Music", "https://www.deezer.com/explore", "streaming", globalStreaming, nil),
		mk("pandora", "Pandora", "https://www.pandora.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("iheartradio", "iHeartRadio", "https://www.iheart.com/", "streaming", append(globalStreaming, "na"), []string{"na"}),
		mk("deepseek", "DeepSeek", "https://chat.deepseek.com/", "ai", globalAI, nil),
		mk("mistral", "Mistral Le Chat", "https://chat.mistral.ai/", "ai", globalAI, nil),
		mk("kimi", "Kimi", "https://www.kimi.com/", "ai", globalAI, nil),
		mk("qwen", "Qwen Chat", "https://chat.qwen.ai/", "ai", globalAI, nil),
		mk("coze", "Coze", "https://www.coze.com/", "ai", globalAI, nil),
		mk("huggingchat", "HuggingChat", "https://huggingface.co/chat/", "ai", globalAI, nil),
		mk("you-com", "You.com", "https://you.com/", "ai", globalAI, nil),
		mk("character-ai", "Character.AI", "https://character.ai/", "ai", globalAI, nil),
		mk("google-ai-studio", "Google AI Studio", "https://aistudio.google.com/", "ai", globalAI, nil),
		mk("notebooklm", "NotebookLM", "https://notebooklm.google.com/", "ai", globalAI, nil),
		mk("microsoft-designer", "Microsoft Designer", "https://designer.microsoft.com/", "ai", globalAI, nil),
		mk("instagram", "Instagram", "https://www.instagram.com/", "social", globalSocial, nil),
		mk("facebook", "Facebook", "https://www.facebook.com/", "social", globalSocial, nil),
		mk("x", "X", "https://x.com/", "social", globalSocial, nil),
		mk("threads", "Threads", "https://www.threads.net/", "social", globalSocial, nil),
		mk("discord", "Discord", "https://discord.com/app", "social", globalSocial, nil),
		mk("snapchat", "Snapchat", "https://www.snapchat.com/", "social", globalSocial, nil),
		mk("hoy-tv-hk", "HOY TV", "https://hoy.tv/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("rthk-hk", "RTHK", "https://www.rthk.hk/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("hbo-go-asia-hk", "HBO GO Asia", "https://www.hbogoasia.hk/", "streaming", []string{"hk"}, []string{"hk"}),
		mk("kktv-tw", "KKTV", "https://www.kktv.me/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("litv-tw", "LiTV", "https://www.litv.tv/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("catchplay-tw", "CATCHPLAY+", "https://www.catchplay.com/tw", "streaming", []string{"tw"}, []string{"tw"}),
		mk("hami-video-tw", "Hami Video", "https://hamivideo.hinet.net/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("myvideo-tw", "MyVideo", "https://www.myvideo.net.tw/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("4gtv-tw", "4GTV", "https://www.4gtv.tv/", "streaming", []string{"tw"}, []string{"tw"}),
		mk("dmm-tv-jp", "DMM TV", "https://tv.dmm.com/vod/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("hulu-jp", "Hulu Japan", "https://www.hulu.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("tver-jp", "TVer", "https://tver.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("lemino-jp", "Lemino", "https://lemino.docomo.ne.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("wowow-jp", "WOWOW", "https://wod.wowow.co.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("danime-jp", "d Anime Store", "https://animestore.docomo.ne.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("videomarket-jp", "VideoMarket", "https://www.videomarket.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("nhk-plus-jp", "NHK Plus", "https://plus.nhk.jp/", "streaming", []string{"jp"}, []string{"jp"}),
		mk("kbs-kr", "KBS", "https://onair.kbs.co.kr/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("naver-tv-kr", "Naver TV", "https://tv.naver.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("kakao-tv-kr", "KakaoTV", "https://tv.kakao.com/", "streaming", []string{"kr"}, []string{"kr"}),
		mk("cbc-gem-na", "CBC Gem", "https://gem.cbc.ca/", "streaming", []string{"na"}, []string{"na"}),
		mk("crave-na", "Crave", "https://www.crave.ca/", "streaming", []string{"na"}, []string{"na"}),
		mk("roku-channel-na", "The Roku Channel", "https://therokuchannel.roku.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("shudder-na", "Shudder", "https://www.shudder.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("fox-na", "FOX", "https://www.fox.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("nbc-na", "NBC", "https://www.nbc.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("abc-na", "ABC", "https://abc.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("cw-na", "The CW", "https://www.cwtv.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("mgm-plus-na", "MGM+", "https://www.mgmplus.com/", "streaming", []string{"na"}, []string{"na"}),
		mk("nfl-plus-na", "NFL+", "https://www.nfl.com/plus/", "sports", []string{"na", "sports"}, []string{"na"}),
		mk("nba-tv-na", "NBA TV", "https://www.nba.com/watch/league-pass-stream", "sports", []string{"na", "sports"}, []string{"na"}),
		mk("bbc-iplayer-eu", "BBC iPlayer", "https://www.bbc.co.uk/iplayer", "streaming", []string{"eu"}, []string{"eu"}),
		mk("itvx-eu", "ITVX", "https://www.itv.com/watch", "streaming", []string{"eu"}, []string{"eu"}),
		mk("channel4-eu", "Channel 4", "https://www.channel4.com/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("channel5-eu", "Channel 5", "https://www.channel5.com/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("zdf-eu", "ZDF", "https://www.zdf.de/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("ard-eu", "ARD Mediathek", "https://www.ardmediathek.de/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("rtl-plus-eu", "RTL+", "https://plus.rtl.de/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("canal-plus-eu", "Canal+", "https://www.canalplus.com/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("molotov-eu", "Molotov", "https://www.molotov.tv/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("raiplay-eu", "RaiPlay", "https://www.raiplay.it/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("npo-start-eu", "NPO Start", "https://npo.nl/start", "streaming", []string{"eu"}, []string{"eu"}),
		mk("skyshowtime-eu", "SkyShowtime", "https://www.skyshowtime.com/", "streaming", []string{"eu"}, []string{"eu"}),
		mk("7plus-oc", "7plus", "https://7plus.com.au/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("10play-oc", "10 Play", "https://10play.com.au/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("abc-iview-oc", "ABC iview", "https://iview.abc.net.au/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("sbs-ondemand-oc", "SBS On Demand", "https://www.sbs.com.au/ondemand/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("tvnz-oc", "TVNZ+", "https://www.tvnz.co.nz/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("neon-oc", "Neon", "https://www.neontv.co.nz/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("maori-tv-oc", "Maori TV", "https://www.maoriplus.co.nz/", "streaming", []string{"oc"}, []string{"oc"}),
		mk("f1-tv", "F1 TV", "https://f1tv.formula1.com/", "sports", globalSports, nil),
		mk("mlb-tv", "MLB.TV", "https://www.mlb.com/live-stream-games", "sports", globalSports, nil),
		mk("nhl-tv", "NHL", "https://www.nhl.com/", "sports", globalSports, nil),
		mk("eurosport", "Eurosport", "https://www.eurosport.com/", "sports", globalSports, nil),
		mk("geforce-now", "GeForce NOW", "https://play.geforcenow.com/", "games", globalGames, nil),
		mk("xbox-cloud", "Xbox Cloud Gaming", "https://www.xbox.com/play", "games", globalGames, nil),
		mk("amazon-luna", "Amazon Luna", "https://luna.amazon.com/", "games", globalGames, nil),
	}
	return providers
}

// Find resolves exact provider IDs and returns a helpful error for omissions.
func Find(ids []string) ([]Provider, error) {
	if len(ids) == 0 {
		return All(), nil
	}
	byID := make(map[string]Provider, len(All()))
	for _, p := range All() {
		byID[p.Definition().ID] = p
	}
	seen := make(map[string]bool)
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		p, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown provider %q", id)
		}
		seen[id] = true
		out = append(out, p)
	}
	return out, nil
}

// Filter selects global providers plus the requested region for regional scopes.
func Filter(providers []Provider, scope string, region string) []Provider {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" || scope == "auto" {
		scope = "auto"
	}
	out := make([]Provider, 0, len(providers))
	for _, p := range providers {
		d := p.Definition()
		if scope == "all" || (scope == "auto" && (has(d.Groups, "global") || (region != "" && has(d.Regions, region)))) || (scope == "global" && has(d.Groups, "global")) || (scope != "all" && scope != "auto" && scope != "global" && (has(d.Groups, scope) || (isRegion(scope) && has(d.Regions, scope)))) {
			out = append(out, p)
		}
	}
	return out
}

func has(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func isRegion(value string) bool {
	switch value {
	case "tw", "hk", "jp", "kr", "na", "sa", "eu", "af", "oc":
		return true
	}
	return false
}
