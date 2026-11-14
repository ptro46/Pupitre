package main

import (
	"errors"
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fhs/gompd/v2/mpd"
)

type track struct {
	id    int
	file  string
	title string
}

type playback struct {
	state        string
	songID       int
	position     int
	volume       int
	elapsed      float64
	duration     float64
	queueVersion string
	sampledAt    time.Time
}

type player struct {
	client    *mpd.Client
	state     playback
	queue     []track
	playlists []string
}

func serverAddress(host string, port int) (string, error) {
	host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
	if host == "" || port < 1 || port > 65535 {
		return "", errors.New("renseigne une adresse et un port entre 1 et 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func connect(address, password string) (*player, error) {
	if strings.ContainsAny(password, "\r\n") {
		return nil, errors.New("le mot de passe doit tenir sur une ligne")
	}
	client, err := mpd.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	p := &player{client: client, state: playback{songID: -1}}
	if password != "" {
		if err := client.Command("password %s", password).OK(); err != nil {
			client.Close()
			return nil, err
		}
	}
	if err := p.refresh(true); err != nil {
		client.Close()
		return nil, err
	}
	return p, nil
}

func (p *player) refresh(listPlaylists bool) error {
	status, err := p.client.Status()
	if err != nil {
		return err
	}
	state := playback{
		state: status["state"], songID: number(status["songid"]), position: number(status["song"]),
		volume: number(status["volume"]), elapsed: seconds(status["elapsed"]), duration: seconds(status["duration"]),
		queueVersion: status["playlist"], sampledAt: time.Now(),
	}
	if state.duration == 0 {
		if elapsed, duration, ok := strings.Cut(status["time"], ":"); ok {
			state.elapsed, state.duration = seconds(elapsed), seconds(duration)
		}
	}
	if p.queue == nil || p.state.queueVersion != state.queueVersion {
		songs, err := p.client.PlaylistInfo(-1, -1)
		if err != nil {
			return err
		}
		p.queue = tracks(songs)
	}
	p.state = state
	if listPlaylists {
		lists, err := p.client.ListPlaylists()
		if err != nil {
			return err
		}
		p.playlists = make([]string, 0, len(lists))
		for _, attrs := range lists {
			p.playlists = append(p.playlists, attrs["playlist"])
		}
	}
	return nil
}

func (p *player) togglePlayback() error {
	switch p.state.state {
	case "play":
		return p.client.Pause(true)
	case "pause":
		return p.client.Pause(false)
	default:
		return p.client.Play(-1)
	}
}

func (p *player) playlistContents(name string) ([]mpd.Attrs, error) {
	// gompd 2.3 formats attribute commands a second time before writing them.
	return p.client.PlaylistContents(strings.ReplaceAll(name, "%", "%%"))
}

var errPlaylistChanged = errors.New("la playlist a changé sur MPD, choisis à nouveau le morceau")

func (p *player) playPlaylist(name string, index int, file string) error {
	songs, err := p.playlistContents(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(songs) || songs[index]["file"] != file {
		return errPlaylistChanged
	}
	if err := p.client.Clear(); err != nil {
		return err
	}
	if err := p.client.PlaylistLoad(name, -1, -1); err != nil {
		return err
	}
	return p.client.Play(index)
}

func tracks(songs []mpd.Attrs) []track {
	items := make([]track, 0, len(songs))
	for _, song := range songs {
		title := song["Title"]
		if title == "" {
			title = path.Base(song["file"])
		}
		if artist := song["Artist"]; artist != "" {
			title = artist + " — " + title
		}
		items = append(items, track{id: number(song["Id"]), file: song["file"], title: title})
	}
	return items
}

func number(value string) int {
	result, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return result
}

func seconds(value string) float64 {
	result, _ := strconv.ParseFloat(value, 64)
	return result
}

func clockLabel(seconds float64) string {
	total := int(seconds)
	if total >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", total/3600, total/60%60, total%60)
	}
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
