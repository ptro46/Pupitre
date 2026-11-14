package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/fhs/gompd/v2/mpd"
)

type view struct {
	window   fyne.Window
	prefs    fyne.Preferences
	player   *player
	busy     bool
	reading  bool
	pending  func(*player) error
	shutdown chan struct{}

	state        playback
	playlists    []string
	preview      []track
	queue        []track
	selectedList string
	selectedID   int

	connection *widget.Label
	connect    *widget.Button
	lists      *widget.List
	tracks     *widget.List
	queued     *widget.List
	previewTab *container.TabItem
	tabs       *container.AppTabs
	playList   *widget.Button
	playTrack  *widget.Button
	nowPlaying *widget.Label
	position   *widget.Label
	clock      *widget.Label
	progress   *widget.ProgressBar
	volume     *widget.Slider
	volumeText *widget.Label
	play       *widget.Button
	stop       *widget.Button
	previous   *widget.Button
	next       *widget.Button
	jumps      []*widget.Button

	updatingVolume bool
	changingVolume bool
}

func main() {
	application := app.NewWithID("io.github.ptro46.pupitre")
	window := application.NewWindow("Pupitre")
	v := &view{
		window: window, prefs: application.Preferences(),
		state: playback{songID: -1, position: -1, volume: -1}, selectedID: -1,
		shutdown: make(chan struct{}),
	}
	window.SetContent(v.content())
	window.Resize(fyne.NewSize(940, 560))
	window.SetOnClosed(func() { close(v.shutdown) })
	go v.tick()
	window.ShowAndRun()
}

func (v *view) content() fyne.CanvasObject {
	v.connection = widget.NewLabel("Déconnecté")
	v.connect = widget.NewButtonWithIcon("Connexion…", theme.SettingsIcon(), v.connectionDialog)
	header := container.NewBorder(nil, nil, v.connect, nil, v.connection)

	v.lists = textList(func() int { return len(v.playlists) }, func(i int) string {
		return v.playlists[i]
	}, nil, nil)
	v.lists.OnSelected = func(i widget.ListItemID) { v.selectPlaylist(v.playlists[i]) }

	v.tracks = textList(func() int { return len(v.preview) }, func(i int) string {
		return v.preview[i].title
	}, nil, func(i int) { v.startPlaylistAt(i) })
	v.playList = widget.NewButtonWithIcon("Lire la playlist", theme.MediaPlayIcon(), func() { v.startPlaylistAt(0) })
	v.previewTab = container.NewTabItem("Playlist", container.NewBorder(nil, v.playList, nil, nil, v.tracks))

	v.queued = textList(func() int { return len(v.queue) }, func(i int) string {
		prefix := "   "
		if v.queue[i].id == v.state.songID {
			prefix = "▶ "
		}
		return fmt.Sprintf("%s%d. %s", prefix, i+1, v.queue[i].title)
	}, func(i int) { v.queued.Select(i) }, func(i int) { v.playQueueTrack(v.queue[i].id) })
	v.queued.OnSelected = func(i widget.ListItemID) {
		v.selectedID = v.queue[i].id
		v.updateButtons()
	}
	v.queued.OnUnselected = func(widget.ListItemID) {
		v.selectedID = -1
		v.updateButtons()
	}
	v.playTrack = widget.NewButton("Lire le morceau sélectionné", func() {
		v.playQueueTrack(v.selectedID)
	})
	queueTab := container.NewTabItem("File de lecture", container.NewBorder(nil, v.playTrack, nil, nil, v.queued))
	v.tabs = container.NewAppTabs(queueTab, v.previewTab)

	sidebar := container.NewBorder(widget.NewLabelWithStyle("Playlists", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, v.lists)
	split := container.NewHSplit(sidebar, v.tabs)
	split.SetOffset(0.28)

	v.nowPlaying = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.nowPlaying.Truncation = fyne.TextTruncateEllipsis
	v.position = widget.NewLabel("")
	v.clock = widget.NewLabel("")
	v.progress = widget.NewProgressBar()
	v.progress.TextFormatter = func() string { return "" }

	v.previous = widget.NewButtonWithIcon("Précédent", theme.MediaSkipPreviousIcon(), func() {
		v.command(func(p *player) error { return p.client.Previous() })
	})
	v.play = widget.NewButtonWithIcon("Lecture", theme.MediaPlayIcon(), func() {
		v.command((*player).togglePlayback)
	})
	v.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		v.command(func(p *player) error { return p.client.Stop() })
	})
	v.next = widget.NewButtonWithIcon("Suivant", theme.MediaSkipNextIcon(), func() {
		v.command(func(p *player) error { return p.client.Next() })
	})
	transport := container.NewHBox(v.previous, v.play, v.stop, v.next)
	jumps := container.NewHBox()
	for _, offset := range []int{-20, -10, 10, 20} {
		button := widget.NewButton(fmt.Sprintf("%+d s", offset), func() {
			v.command(func(p *player) error { return p.client.SeekCur(time.Duration(offset)*time.Second, true) })
		})
		v.jumps = append(v.jumps, button)
		jumps.Add(button)
	}
	v.volumeText = widget.NewLabel("")
	v.volume = widget.NewSlider(0, 100)
	v.volume.OnChanged = func(value float64) {
		if !v.updatingVolume {
			v.changingVolume = true
			v.volumeText.SetText(fmt.Sprintf("Volume %d %%", int(value)))
		}
	}
	v.volume.OnChangeEnded = func(value float64) {
		if v.updatingVolume {
			return
		}
		v.changingVolume = false
		v.command(func(p *player) error { return p.client.SetVolume(int(value)) })
	}
	volume := container.NewBorder(nil, nil, v.volumeText, nil, v.volume)
	playerPanel := container.NewVBox(
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, v.position, v.nowPlaying),
		container.NewBorder(nil, nil, nil, v.clock, v.progress),
		container.NewBorder(nil, nil, transport, jumps),
		volume,
	)
	v.renderPlayback()
	return container.NewPadded(container.NewBorder(header, playerPanel, nil, nil, split))
}

type trackRow struct {
	widget.Label
	onTap       func()
	onDoubleTap func()
}

func (r *trackRow) Tapped(*fyne.PointEvent) {
	if r.onTap != nil {
		r.onTap()
	}
}

func (r *trackRow) DoubleTapped(*fyne.PointEvent) {
	r.onDoubleTap()
}

func textList(length func() int, label func(int) string, tap, doubleTap func(int)) *widget.List {
	return widget.NewList(length, func() fyne.CanvasObject {
		if doubleTap == nil {
			item := widget.NewLabel("")
			item.Truncation = fyne.TextTruncateEllipsis
			return item
		}
		row := &trackRow{}
		row.Truncation = fyne.TextTruncateEllipsis
		row.ExtendBaseWidget(row)
		return row
	}, func(i widget.ListItemID, object fyne.CanvasObject) {
		if row, ok := object.(*trackRow); ok {
			if tap != nil {
				row.onTap = func() { tap(i) }
			}
			row.onDoubleTap = func() { doubleTap(i) }
			row.SetText(label(i))
			return
		}
		object.(*widget.Label).SetText(label(i))
	})
}

func (v *view) connectionDialog() {
	if v.player != nil {
		v.disconnect()
		return
	}

	host := widget.NewEntry()
	host.SetText(v.prefs.StringWithFallback("mpd-host", "localhost"))
	port := widget.NewEntry()
	port.SetText(v.prefs.StringWithFallback("mpd-port", "6600"))
	password := widget.NewPasswordEntry()
	form := dialog.NewForm("Connexion à MPD", "Connecter", "Annuler", []*widget.FormItem{
		widget.NewFormItem("Serveur", host),
		widget.NewFormItem("Port TCP", port),
		widget.NewFormItem("Mot de passe (facultatif)", password),
	}, func(confirmed bool) {
		if !confirmed {
			return
		}
		hostValue, portValue, secret := host.Text, port.Text, password.Text
		address, err := serverAddress(hostValue, number(portValue))
		if err != nil {
			dialog.ShowError(err, v.window)
			return
		}
		v.busy = true
		v.connection.SetText("Connexion à " + address + "…")
		v.updateButtons()
		go func() {
			p, err := connect(address, secret)
			fyne.Do(func() {
				v.busy = false
				if err != nil {
					v.connection.SetText("Déconnecté")
					v.updateButtons()
					dialog.ShowError(fmt.Errorf("connexion à %s : %w", address, err), v.window)
					return
				}
				v.player = p
				v.prefs.SetString("mpd-host", hostValue)
				v.prefs.SetString("mpd-port", portValue)
				v.connection.SetText("Connecté à " + address)
				v.connect.SetText("Déconnexion")
				v.syncPlayer()
			})
		}()
	}, v.window)
	form.Resize(fyne.NewSize(425, 235))
	form.Show()
}

func (v *view) disconnect() {
	p := v.player
	reading := v.reading
	v.reading = false
	v.player = nil
	v.pending = nil
	v.selectedList = ""
	v.lists.UnselectAll()
	v.queued.UnselectAll()
	v.selectedID = -1
	v.preview, v.playlists, v.queue = nil, nil, nil
	v.state = playback{songID: -1, position: -1, volume: -1}
	v.changingVolume = false
	v.lists.Refresh()
	v.tracks.Refresh()
	v.queued.Refresh()
	v.previewTab.Text = "Playlist"
	v.tabs.Refresh()
	v.tabs.SelectIndex(0)
	v.connection.SetText("Déconnecté")
	v.connect.SetText("Connexion…")
	v.renderPlayback()
	if !reading {
		go p.client.Close()
	}
}

func (v *view) selectPlaylist(name string) {
	v.selectedList = name
	v.preview = nil
	v.tracks.Refresh()
	v.previewTab.Text = name
	v.tabs.Refresh()
	v.tabs.Select(v.previewTab)
	v.updateButtons()
}

func (v *view) playQueueTrack(id int) {
	if id >= 0 {
		v.command(func(p *player) error { return p.client.PlayID(id) })
	}
}

func (v *view) startPlaylistAt(index int) {
	if v.player == nil || v.busy || v.pending != nil || index < 0 || index >= len(v.preview) {
		return
	}
	name := v.selectedList
	file := v.preview[index].file
	start := func() {
		v.command(func(p *player) error { return p.playPlaylist(name, index, file) })
		v.tabs.SelectIndex(0)
	}
	if len(v.queue) == 0 {
		start()
		return
	}
	dialog.ShowConfirm("Remplacer la file de lecture ?", "La playlist « "+name+" » remplacera les morceaux actuellement dans la file.", func(confirmed bool) {
		if confirmed {
			start()
		}
	}, v.window)
}

func (v *view) command(action func(*player) error) {
	if v.player == nil || v.busy || v.pending != nil {
		return
	}
	if v.reading {
		v.pending = action
		v.updateButtons()
		return
	}
	p := v.player
	v.busy = true
	v.updateButtons()
	go func() {
		err := action(p)
		refreshErr := p.refresh(false)
		fyne.Do(func() {
			v.busy = false
			if refreshErr != nil {
				v.disconnect()
				v.connection.SetText("Connexion perdue")
				if err == nil {
					err = refreshErr
				}
			} else {
				if errors.Is(err, errPlaylistChanged) {
					v.preview = nil
					v.tracks.Refresh()
					v.tabs.Select(v.previewTab)
				}
				v.syncPlayer()
			}
			if err != nil {
				dialog.ShowError(err, v.window)
			}
		})
	}()
}

func (v *view) tick() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-v.shutdown:
			return
		case <-ticker.C:
			fyne.Do(v.update)
		}
	}
}

func (v *view) update() {
	v.renderClock()
	if v.player == nil || v.busy || v.reading || v.changingVolume {
		return
	}
	name := v.selectedList
	refresh := time.Since(v.state.sampledAt) >= time.Second
	loadPreview := name != "" && v.preview == nil
	if !refresh && !loadPreview {
		return
	}
	p := v.player
	v.reading = true
	go func() {
		var err error
		var preview []track
		if refresh {
			err = p.refresh(true)
		}
		if err == nil && loadPreview {
			var songs []mpd.Attrs
			songs, err = p.playlistContents(name)
			preview = tracks(songs)
		}
		fyne.Do(func() {
			if v.player != p {
				go p.client.Close()
				return
			}
			v.reading = false
			if err != nil {
				var serverError mpd.Error
				if loadPreview && errors.As(err, &serverError) && serverError.CommandName == "listplaylistinfo" {
					if v.selectedList == name {
						v.preview = []track{}
					}
					dialog.ShowError(err, v.window)
				} else {
					v.disconnect()
					v.connection.SetText("Connexion perdue")
					dialog.ShowError(err, v.window)
					return
				}
			}
			if loadPreview && err == nil && v.selectedList == name {
				v.preview = preview
				v.tracks.Refresh()
			}
			v.syncPlayer()
			if action := v.pending; action != nil {
				v.pending = nil
				v.command(action)
			}
		})
	}()
}

func (v *view) syncPlayer() {
	p := v.player
	queueChanged := v.state.queueVersion != p.state.queueVersion || len(v.queue) != len(p.queue)
	songChanged := v.state.songID != p.state.songID
	v.state = p.state
	if !sameNames(v.playlists, p.playlists) {
		v.lists.UnselectAll()
		v.playlists = p.playlists
		v.selectedList, v.preview = "", nil
		v.previewTab.Text = "Playlist"
		v.tabs.Refresh()
		v.tracks.Refresh()
		v.lists.Refresh()
	}
	if queueChanged {
		v.queued.UnselectAll()
		v.queue = p.queue
	}
	if queueChanged || songChanged {
		v.queued.Refresh()
		if v.state.position >= 0 && v.state.position < len(v.queue) {
			v.queued.ScrollTo(v.state.position)
		}
	}
	v.renderPlayback()
}

func sameNames(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for i, name := range first {
		if name != second[i] {
			return false
		}
	}
	return true
}

func (v *view) renderPlayback() {
	title, position := "Aucun morceau", ""
	if i := v.state.position; v.player != nil && i >= 0 && i < len(v.queue) {
		title = v.queue[i].title
		position = fmt.Sprintf("Morceau %d / %d", i+1, len(v.queue))
	}
	v.nowPlaying.SetText(title)
	v.position.SetText(position)
	if v.state.state == "play" && v.play.Text != "Pause" {
		v.play.SetText("Pause")
		v.play.SetIcon(theme.MediaPauseIcon())
	} else if v.state.state != "play" && v.play.Text != "Lecture" {
		v.play.SetText("Lecture")
		v.play.SetIcon(theme.MediaPlayIcon())
	}
	if v.state.volume < 0 {
		v.volumeText.SetText("Volume indisponible")
	} else if !v.changingVolume {
		v.updatingVolume = true
		v.volume.SetValue(float64(v.state.volume))
		v.volumeText.SetText("Volume " + strconv.Itoa(v.state.volume) + " %")
		v.updatingVolume = false
	}
	v.renderClock()
	v.updateButtons()
}

func (v *view) renderClock() {
	elapsed := v.state.elapsed
	if v.state.state == "play" {
		elapsed += time.Since(v.state.sampledAt).Seconds()
	}
	duration := v.state.duration
	if duration > 0 {
		elapsed = min(elapsed, duration)
		v.progress.SetValue(elapsed / duration)
		v.clock.SetText(clockLabel(elapsed) + " / " + clockLabel(duration))
		return
	}
	v.progress.SetValue(0)
	v.clock.SetText(clockLabel(elapsed))
}

func (v *view) updateButtons() {
	connected := v.player != nil
	canControl := connected && !v.busy && v.pending == nil
	hasSong := canControl && v.state.songID >= 0
	enable(v.connect, !v.busy)
	enable(v.playList, canControl && len(v.preview) > 0)
	enable(v.playTrack, canControl && v.selectedID >= 0)
	for _, button := range []*widget.Button{v.previous, v.play, v.stop, v.next} {
		enable(button, canControl && len(v.queue) > 0)
	}
	for _, button := range v.jumps {
		enable(button, hasSong && v.state.state != "stop" && v.state.duration > 0)
	}
	enable(v.volume, canControl && v.state.volume >= 0)
}

func enable(item fyne.Disableable, enabled bool) {
	if enabled == !item.Disabled() {
		return
	}
	if enabled {
		item.Enable()
	} else {
		item.Disable()
	}
}
