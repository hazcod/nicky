package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultRtmpPath = "/usr/local/bin/rtmpdump"
)

var (
	sesRegex = regexp.MustCompile(`Seizoen (\d+)`)
	epiRegex = regexp.MustCompile(`Aflevering (\d+)`)
)

type Episode struct {
	Url   string
	Title string
	Show  string
	//------------
	Season  int
	Episode int
}

type Playlist struct {
	XMLName xml.Name `xml:"rss"`
	Text    string   `xml:",chardata"`
	Media   string   `xml:"media,attr"`
	Mediaad string   `xml:"mediaad,attr"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Text        string `xml:",chardata"`
		Description string `xml:"description"`
		Item        struct {
			Text        string `xml:",chardata"`
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			AirTime     string `xml:"airTime"`
			PubDate     string `xml:"pubDate"`
			EpisodeOid  string `xml:"episodeOid"`
			Description string `xml:"description"`
			Guid        struct {
				Text        string `xml:",chardata"`
				IsPermaLink string `xml:"isPermaLink,attr"`
			} `xml:"guid"`
			Group struct {
				Text    string `xml:",chardata"`
				Content struct {
					Text      string `xml:",chardata"`
					Duration  string `xml:"duration,attr"`
					IsDefault string `xml:"isDefault,attr"`
					Type      string `xml:"type,attr"`
					URL       string `xml:"url,attr"`
				} `xml:"content"`
				Player struct {
					Text string `xml:",chardata"`
					URL  string `xml:"url,attr"`
				} `xml:"player"`
				Category []struct {
					Text   string `xml:",chardata"`
					Scheme string `xml:"scheme,attr"`
				} `xml:"category"`
			} `xml:"group"`
		} `xml:"item"`
	} `xml:"channel"`
}

type PlaylistStreams struct {
	XMLName xml.Name `xml:"package"`
	Text    string   `xml:",chardata"`
	Version string   `xml:"version,attr"`
	Video   struct {
		Text string `xml:",chardata"`
		Item struct {
			Text      string `xml:",chardata"`
			Rendition []struct {
				Text     string `xml:",chardata"`
				Cdn      string `xml:"cdn,attr"`
				Duration string `xml:"duration,attr"`
				Width    string `xml:"width,attr"`
				Height   string `xml:"height,attr"`
				Type     string `xml:"type,attr"`
				Bitrate  string `xml:"bitrate,attr"`
				Src      string `xml:"src"`
			} `xml:"rendition"`
		} `xml:"item"`
	} `xml:"video"`
}

func fetchPage(url string) (body string, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.New("could not fetch " + url + " : " + err.Error())
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return "", errors.New(fmt.Sprintf("url %s returned statusCode %d", url, resp.StatusCode))
	}

	html, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New(fmt.Sprintf("could not read body of %s : %s", url, err))
	}

	return string(html), nil
}

func extractEpisodes(contents string) (episodes []Episode, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contents))
	if err != nil {
		return nil, errors.New("could not parse episodes page: " + err.Error())
	}

	doc.Find("li.fullepisode.playlist-item").Each(func(i int, s *goquery.Selection) {
		episodeUrl, urlFound := s.Find("a").Attr("href")
		if !urlFound {
			log.Printf("warning: no episode url found for %s", s.Find("a").Text())
			return
		}

		episodeTitle, titleFound := s.Find("img").Attr("title")
		if !titleFound {
			log.Printf("warning: no title found for %s", s.Find("img"))
			return
		}

		showName := s.Find("p.title").Text()
		if showName == "" {
			log.Print("warning: could not extract show name")
			return
		}

		episodes = append(episodes, Episode{
			Url:   episodeUrl,
			Title: episodeTitle,
			Show:  showName,
		})
	})

	return
}

func extractEpisodeNumbering(contents string) (season int, episode int, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contents))
	if err != nil {
		return -1, -1, errors.New("could not parse episode page: " + err.Error())
	}

	episodeText := strings.TrimSpace(doc.Find("h6.season-episode").Text())
	if episodeText == "" {
		return -1, -1, errors.New("missing season/episode")
	}

	episodeParts := epiRegex.FindStringSubmatch(episodeText)
	if len(episodeParts) != 2 {
		return -1, -1, errors.New("could not parse episode parts: " + episodeText)
	}
	episode, err = strconv.Atoi(episodeParts[1])
	if err != nil {
		return -1, -1, errors.New("invalid episode number: " + episodeParts[1])
	}

	seasonParts := sesRegex.FindStringSubmatch(episodeText)
	if len(seasonParts) != 2 {
		return -1, -1, errors.New("could not parse seasonparts: " + episodeText)
	}
	season, err = strconv.Atoi(seasonParts[1])
	if err != nil {
		return -1, -1, errors.New("invalid season number: " + seasonParts[1])
	}

	return
}

func extractPlaylist(contents string) (playlist Playlist, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contents))
	if err != nil {
		return Playlist{}, errors.New("could not parse for playlist url: " + err.Error())
	}

	playlistUrl, exists := doc.Find("div.player-wrapper").Attr("data-mrss")
	if playlistUrl == "" || !exists {
		return Playlist{}, errors.New("could not extract playlist url")
	}

	playlistPage, err := fetchPage(playlistUrl)
	if err != nil {
		return Playlist{}, errors.New("could not fetch playlist " + playlistUrl)
	}

	if err := xml.Unmarshal([]byte(playlistPage), &playlist); err != nil {
		return Playlist{}, errors.New("could not marshal playlist: " + err.Error())
	}

	return playlist, nil
}

func extractBestQualityStream(playlist Playlist) (url string, err error) {
	url = playlist.Channel.Item.Group.Content.URL
	if url == "" {
		return "", errors.New("could not parse media:content url")
	}

	streamsPage, err := fetchPage(url)
	if err != nil {
		return "", errors.New("could not fetch streams page: " + err.Error())
	}

	var streams PlaylistStreams
	if err := xml.Unmarshal([]byte(streamsPage), &streams); err != nil {
		return "", errors.New("could not unmarshal streamsPage: " + err.Error())
	}

	if len(streams.Video.Item.Rendition) == 0 {
		return "", errors.New("no renditions found in bitrate")
	}

	bestRendition := streams.Video.Item.Rendition[0]

	for i, rendition := range streams.Video.Item.Rendition {
		if i == 0 {
			continue
		}

		if rendition.Bitrate > bestRendition.Bitrate {
			bestRendition = rendition
		}
	}

	if bestRendition.Src == "" {
		return "", errors.New("empty rendition stream url")
	}

	return bestRendition.Src, nil
}

func ScrapeRTMP(rtmpPath string, rtmpUrl string, destPath string) (err error) {
	//"/usr/local/bin/rtmpdump --url " + shlex.quote(source) + " -o  " + tempFile
	f, err := ioutil.TempFile("/tmp/", "nicky")
	if err != nil {
		return errors.New("could not create temp file: %v" + err.Error())
	}

	defer f.Close()
	defer os.Remove(f.Name())

	cmd := exec.Command(rtmpPath, "--quiet", "--url", rtmpUrl, "--flv", f.Name())
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.New("could not run rtmpdump: %v" + err.Error())
	}

	if err := os.Rename(f.Name(), destPath); err != nil {
		return errors.New("could not move file: %v" + err.Error())
	}

	return nil
}

func main() {
	var website string
	flag.StringVar(&website, "site", "nickelodeon.be", "The website to scrape")
	var showId string
	flag.StringVar(&showId, "show", "", "The show slug taken from the website")
	var mediaPath string
	flag.StringVar(&mediaPath, "path", "", "The directory where to store the files")
	var rtmpPath string
	flag.StringVar(&rtmpPath, "rtmp", defaultRtmpPath, "The path to the rtmpdump binary")
	flag.Parse()

	if showId == "" || website == "" || mediaPath == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	showPath := url.URL{
		Scheme: "http",
		Host:   website,
		Path:   fmt.Sprintf("/%s", showId),
	}

	log.Printf("Scraping show %s", showPath.String())
	mainPage, err := fetchPage(showPath.String())
	if err != nil {
		log.Fatalf("could not fetch show page %s: %v", showPath.String(), err)
	}

	log.Print("extracting episodes")
	episodes, err := extractEpisodes(mainPage)
	if err != nil {
		log.Fatalf("could not extract episodes: %v", err)
	}

	log.Printf("found %d episodes for %s", len(episodes), showId)

	for _, epi := range episodes {
		log.Printf("Downloading %s %s", epi.Show, epi.Title)

		page, err := fetchPage(epi.Url)
		if err != nil {
			log.Printf("error: could not fetch episode page: %v", err)
			continue
		}

		epi.Season, epi.Episode, err = extractEpisodeNumbering(page)
		if err != nil {
			log.Printf("error: could not extract episode numbering: %v", err)
			continue
		}

		destPath := filepath.Join(
			mediaPath,
			fmt.Sprintf("%s/", epi.Show),
			fmt.Sprintf("%s - %s - S%02dE%02d.mp4", epi.Show, epi.Title, epi.Season, epi.Episode),
		)
		log.Printf("will be saved to %s", destPath)

		if _, err := os.Stat(destPath); !os.IsNotExist(err) {
			log.Printf("file already exists")
			continue
		}

		playlist, err := extractPlaylist(page)
		if err != nil {
			log.Printf("error: could not extract playlist: %v", err)
			continue
		}

		streamUrl, err := extractBestQualityStream(playlist)
		if err != nil {
			log.Printf("error: could not extract best stream: %v", err)
			continue
		}

		log.Printf("best stream: %s", streamUrl)

		if err := ScrapeRTMP(rtmpPath, streamUrl, destPath); err != nil {
			log.Printf("could not scrape rtmp to %s: %v", destPath, err)
			continue
		}
	}
}
