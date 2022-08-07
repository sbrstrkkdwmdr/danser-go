package launcher

/*
#ifdef _WIN32

#include <windows.h>
#include <winuser.h>

void beep_custom() {
	MessageBeep(0x00000000L);
}
void beep_error() {
	MessageBeep(0x00000030L);
}

#else

void beep_custom() {}

void beep_error() {}

#endif
*/
import "C"
import (
	"bufio"
	"errors"
	"fmt"
	"github.com/faiface/mainthread"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/inkyblackness/imgui-go/v4"
	"github.com/sqweek/dialog"
	"github.com/wieku/danser-go/app/beatmap"
	"github.com/wieku/danser-go/app/database"
	"github.com/wieku/danser-go/app/graphics"
	"github.com/wieku/danser-go/app/input"
	"github.com/wieku/danser-go/app/settings"
	"github.com/wieku/danser-go/app/states/components/common"
	"github.com/wieku/danser-go/build"
	"github.com/wieku/danser-go/framework/assets"
	"github.com/wieku/danser-go/framework/bass"
	"github.com/wieku/danser-go/framework/env"
	"github.com/wieku/danser-go/framework/goroutines"
	"github.com/wieku/danser-go/framework/graphics/batch"
	"github.com/wieku/danser-go/framework/graphics/viewport"
	"github.com/wieku/danser-go/framework/math/animation"
	"github.com/wieku/danser-go/framework/math/animation/easing"
	"github.com/wieku/danser-go/framework/math/mutils"
	"github.com/wieku/danser-go/framework/math/vector"
	"github.com/wieku/danser-go/framework/platform"
	"github.com/wieku/danser-go/framework/qpc"
	"github.com/wieku/danser-go/framework/util"
	"github.com/wieku/rplpa"
	"golang.org/x/exp/slices"
	"image"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Mode int

const (
	CursorDance Mode = iota
	DanserReplay
	Replay
	Knockout
	NewKnockout
	Play
)

func (m Mode) String() string {
	switch m {
	case CursorDance:
		return "Cursor dance / mandala / tag"
	case DanserReplay:
		return "Cursor dance with UI"
	case Replay:
		return "Watch a replay"
	case Knockout:
		return "Watch knockout (classic)"
	case NewKnockout:
		return "Watch knockout"
	case Play:
		return "Play osu!standard"
	}

	return ""
}

var modes = []Mode{CursorDance, DanserReplay, Replay, Knockout, NewKnockout, Play}

type PMode int

const (
	Watch PMode = iota
	Record
	Screenshot
)

func (m PMode) String() string {
	switch m {
	case Watch:
		return "Watch"
	case Record:
		return "Record"
	case Screenshot:
		return "Screenshot"
	}

	return ""
}

var pModes = []PMode{Watch, Record, Screenshot}

type ConfigMode int

const (
	Rename ConfigMode = iota
	Clone
	New
)

type launcher struct {
	win *glfw.Window

	bg *common.Background

	batch *batch.QuadBatch
	coin  *common.DanserCoin

	bld *builder

	beatmaps []*beatmap.BeatMap

	configList    []string
	currentConfig *settings.Config

	newDefault bool

	mapsLoaded bool

	newCloneOpened bool

	configManiMode ConfigMode
	configPrevName string

	newCloneName     string
	refreshRate      int
	configEditOpened bool
	configEditPos    imgui.Vec2

	danserRunning       bool
	recordProgress      float32
	recordStatus        string
	recordStatusSpeed   string
	recordStatusElapsed string
	recordStatusETA     string
	showProgressBar     bool

	triangleSpeed    *animation.Glider
	encodeInProgress bool
	encodeStart      time.Time
	danserCmd        *exec.Cmd
	popupStack       []iPopup

	selectWindow *songSelectPopup
	splashText   string

	prevMap *beatmap.BeatMap

	configSearch string

	lastReplayDir   string
	lastKnockoutDir string
}

func StartLauncher() {
	defer func() {
		var err any
		var stackTrace []string

		if err = recover(); err != nil {
			stackTrace = goroutines.GetStackTrace(4)
		}

		closeHandler(err, stackTrace)
	}()

	goroutines.SetCrashHandler(closeHandler)

	launcher := &launcher{
		bld:        newBuilder(),
		popupStack: make([]iPopup, 0),
	}

	file, err := os.Create(filepath.Join(env.DataDir(), "launcher.log"))
	if err != nil {
		panic(err)
	}

	log.SetOutput(io.MultiWriter(os.Stdout, file))

	log.Println("danser-go version:", build.VERSION)

	loadLauncherConfig()

	settings.CreateDefault()

	settings.Playfield.Background.Triangles.Enabled = true
	settings.Playfield.Background.Triangles.DrawOverBlur = true
	settings.Playfield.Background.Blur.Enabled = false
	settings.Playfield.Background.Parallax.Enabled = false

	assets.Init(build.Stream == "Dev")

	mainthread.Run(func() {
		defer func() {
			if err := recover(); err != nil {
				stackTrace := goroutines.GetStackTrace(4)
				closeHandler(err, stackTrace)
			}
		}()

		mainthread.Call(launcher.startGLFW)

		for !launcher.win.ShouldClose() {
			mainthread.Call(func() {
				if launcher.win.GetAttrib(glfw.Iconified) == glfw.False {
					if launcher.win.GetAttrib(glfw.Focused) == glfw.False {
						glfw.SwapInterval(2)
					} else {
						glfw.SwapInterval(1)
					}
				} else {
					glfw.SwapInterval(launcher.refreshRate / 10)
				}
				launcher.Draw()
				launcher.win.SwapBuffers()
				glfw.PollEvents()
			})
		}
	})

	// Save configs on exit
	saveLauncherConfig()
	if launcher.currentConfig != nil {
		launcher.currentConfig.Save("", false)
	}
}

func closeHandler(err any, stackTrace []string) {
	if err != nil {
		log.Println("panic:", err)

		for _, s := range stackTrace {
			log.Println(s)
		}

		showMessage(mError, "Launcher crashed with message:\n %s", err)

		os.Exit(1)
	}

	log.Println("Exiting normally.")
}

func (l *launcher) startGLFW() {
	err := glfw.Init()
	if err != nil {
		panic("Failed to initialize GLFW: " + err.Error())
	}

	l.refreshRate = glfw.GetPrimaryMonitor().GetVideoMode().RefreshRate

	l.tryCreateDefaultConfig()
	l.createConfigList()
	settings.LoadCredentials()

	l.bld.config = *launcherConfig.Profile

	c, err := l.loadConfig(l.bld.config)
	if err != nil {
		showMessage(mError, "Failed to read \"%s\" profile.\nReverting to \"default\".\nError: %s", l.bld.config, err)

		l.bld.config = "default"
		*launcherConfig.Profile = l.bld.config
		saveLauncherConfig()

		c, err = l.loadConfig(l.bld.config)
		if err != nil {
			panic(err)
		}
	}

	settings.General.OsuSongsDir = c.General.OsuSongsDir

	l.currentConfig = c

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)
	glfw.WindowHint(glfw.Samples, 4)

	settings.Graphics.Fullscreen = false
	settings.Graphics.WindowWidth = 800
	settings.Graphics.WindowHeight = 534

	l.win, err = glfw.CreateWindow(800, 534, "danser-go "+build.VERSION+" launcher", nil, nil)

	if err != nil {
		panic(err)
	}

	input.Win = l.win

	icon, eee := assets.GetPixmap("assets/textures/dansercoin.png")
	if eee != nil {
		log.Println(eee)
	}
	icon2, _ := assets.GetPixmap("assets/textures/dansercoin48.png")
	icon3, _ := assets.GetPixmap("assets/textures/dansercoin24.png")
	icon4, _ := assets.GetPixmap("assets/textures/dansercoin16.png")

	l.win.SetIcon([]image.Image{icon.NRGBA(), icon2.NRGBA(), icon3.NRGBA(), icon4.NRGBA()})

	icon.Dispose()
	icon2.Dispose()
	icon3.Dispose()
	icon4.Dispose()

	l.win.MakeContextCurrent()

	log.Println("GLFW initialized!")

	gl.Init()

	extensionCheck()

	glfw.SwapInterval(1)

	SetupImgui(l.win)

	graphics.LoadTextures()

	bass.Init(false)

	l.triangleSpeed = animation.NewGlider(1)
	l.triangleSpeed.SetEasing(easing.OutQuad)

	l.batch = batch.NewQuadBatch()

	l.bg = common.NewBackground(false)

	settings.Playfield.Background.Triangles.Enabled = true

	l.coin = common.NewDanserCoin()

	l.coin.DrawVisualiser(true)

	goroutines.RunOS(func() {
		l.loadBeatmaps()

		if launcherConfig.CheckForUpdates {
			checkForUpdates(false)
		}
	})
}

func (l *launcher) loadBeatmaps() {
	l.splashText = "Loading maps...\nThis may take a while..."

	l.beatmaps = make([]*beatmap.BeatMap, 0)

	err := database.Init()
	if err != nil {
		showMessage(mError, "Failed to initialize database! Error: %s\nMake sure Song's folder does exist or change it to the correct directory in settings.", err)
		l.beatmaps = make([]*beatmap.BeatMap, 0)
	} else {
		bSplash := "Loading maps...\nThis may take a while...\n\n"

		beatmaps := database.LoadBeatmaps(launcherConfig.SkipMapUpdate, func(stage database.ImportStage, processed, target int) {
			switch stage {
			case database.Discovery:
				l.splashText = bSplash + "Searching for .osu files...\n\n"
			case database.Comparison:
				l.splashText = bSplash + "Comparing files with database...\n\n"
			case database.Cleanup:
				l.splashText = bSplash + "Removing leftover maps from database...\n\n"
			case database.Import:
				percent := float64(processed) / float64(target) * 100
				l.splashText = bSplash + fmt.Sprintf("Importing maps...\n%d / %d\n%.0f%%", processed, target, percent)
			case database.Finished:
				l.splashText = bSplash + "Finished!\n\n"
			}
		})

		slices.SortFunc(beatmaps, func(a, b *beatmap.BeatMap) bool {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})

		bSplash = "Calculating Star Rating...\nThis may take a while...\n\n\n"

		l.splashText = bSplash + "\n"

		database.UpdateStarRating(beatmaps, func(processed, target int) {
			percent := float64(processed) / float64(target) * 100
			l.splashText = bSplash + fmt.Sprintf("%d / %d\n%.0f%%", processed, target, percent)
		})

		for _, bMap := range beatmaps {
			l.beatmaps = append(l.beatmaps, bMap)
		}

		//database.Close()
	}

	l.win.SetDropCallback(func(w *glfw.Window, names []string) {
		if len(names) > 1 {
			l.trySelectReplaysFromPaths(names)
		} else {
			l.trySelectReplayFromPath(names[0])
		}
	})

	l.win.SetCloseCallback(func(w *glfw.Window) {
		if l.danserCmd != nil {
			l.win.SetShouldClose(false)

			goroutines.Run(func() {
				if showMessage(mQuestion, "Recording is in progress, do you want to exit?") {
					if l.danserCmd != nil {
						l.danserCmd.Process.Kill()
						l.danserCleanup(false)
					}

					l.win.SetShouldClose(true)
				}
			})
		}
	})

	if len(os.Args) > 2 { //won't work in combined mode
		l.trySelectReplaysFromPaths(os.Args[1:])
	} else if len(os.Args) > 1 {
		l.trySelectReplayFromPath(os.Args[1])
	} else if launcherConfig.LoadLatestReplay {
		l.loadLatestReplay()
	}

	l.mapsLoaded = true
}

func (l *launcher) loadLatestReplay() {
	replaysDir := l.currentConfig.General.GetReplaysDir()

	type lastModPath struct {
		tStamp time.Time
		name   string
	}

	var list []*lastModPath

	entries, err := os.ReadDir(replaysDir)
	if err != nil {
		return
	}

	for _, d := range entries {
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".osr") {
			if info, err1 := d.Info(); err1 == nil {
				list = append(list, &lastModPath{
					tStamp: info.ModTime(),
					name:   d.Name(),
				})
			}
		}
	}

	if list == nil {
		return
	}

	slices.SortFunc(list, func(a, b *lastModPath) bool {
		return a.tStamp.After(b.tStamp)
	})

	// Load the newest that can be used
	for _, lMP := range list {
		r, err := l.loadReplay(filepath.Join(replaysDir, lMP.name))
		if err == nil {
			l.trySelectReplay(r)
			break
		}
	}
}

func extensionCheck() {
	extensions := []string{
		"GL_ARB_clear_texture",
		"GL_ARB_direct_state_access",
		"GL_ARB_texture_storage",
		"GL_ARB_vertex_attrib_binding",
		"GL_ARB_buffer_storage",
	}

	var notSupported []string

	for _, ext := range extensions {
		if !glfw.ExtensionSupported(ext) {
			notSupported = append(notSupported, ext)
		}
	}

	if len(notSupported) > 0 {
		panic(fmt.Sprintf("Your GPU does not support one or more required OpenGL extensions: %s. Please update your graphics drivers or upgrade your GPU", notSupported))
	}
}

func (l *launcher) Draw() {
	w, h := l.win.GetFramebufferSize()
	viewport.Push(w, h)

	if l.bg.HasBackground() {
		gl.ClearColor(0, 0, 0, 1.0)
	} else {
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
	}

	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Enable(gl.SCISSOR_TEST)

	w, h = int(settings.Graphics.WindowWidth), int(settings.Graphics.WindowHeight)

	settings.Graphics.Fullscreen = false
	settings.Graphics.WindowWidth = int64(w)
	settings.Graphics.WindowHeight = int64(h)

	if l.currentConfig != nil {
		settings.Audio.GeneralVolume = l.currentConfig.Audio.GeneralVolume
		settings.Audio.MusicVolume = l.currentConfig.Audio.MusicVolume
	}

	t := qpc.GetMilliTimeF()

	l.triangleSpeed.Update(t)

	settings.Playfield.Background.Triangles.Speed = l.triangleSpeed.GetValue()

	l.bg.Update(t, 0, 0)

	l.batch.SetCamera(mgl32.Ortho(-float32(w)/2, float32(w)/2, float32(h)/2, -float32(h)/2, -1, 1))

	bgA := 1.0
	if l.bg.HasBackground() {
		bgA = 0.33
	}

	l.bg.Draw(t, l.batch, 0, bgA, l.batch.Projection)

	l.batch.SetColor(1, 1, 1, 1)
	l.batch.ResetTransform()
	l.batch.SetCamera(mgl32.Ortho(0, float32(w), float32(h), 0, -1, 1))

	l.batch.Begin()

	if l.mapsLoaded {
		if l.bld.currentMap != nil && l.prevMap != l.bld.currentMap {
			l.bg.SetBeatmap(l.bld.currentMap, false, false)

			l.prevMap = l.bld.currentMap
		}

		if l.selectWindow != nil {
			if l.selectWindow.PreviewedSong != nil {
				l.selectWindow.PreviewedSong.Update()
				l.coin.SetMap(l.selectWindow.prevMap, l.selectWindow.PreviewedSong)
				l.bg.SetTrack(l.selectWindow.PreviewedSong)
			} else {
				l.coin.SetMap(nil, nil)
				l.bg.SetTrack(nil)
			}
		}

		l.coin.SetPosition(vector.NewVec2d(468+155.5, 180+85))
		l.coin.SetScale(float64(h) / 4)
		l.coin.SetRotation(0.1)

		l.coin.Update(t)
		l.coin.Draw(t, l.batch)
	}

	l.batch.End()

	l.drawImgui()

	viewport.Pop()
}

func (l *launcher) drawImgui() {
	Begin()

	lock := l.danserRunning

	if lock {
		imgui.PushItemFlag(imgui.ItemFlagsDisabled, true)
	}

	wW, wH := int(settings.Graphics.WindowWidth), int(settings.Graphics.WindowHeight)

	imgui.SetNextWindowSize(vec2(float32(wW), float32(wH)))

	imgui.SetNextWindowPos(vzero())

	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, vec2(20, 20))

	imgui.BeginV("main", nil, imgui.WindowFlagsNoDecoration /*|imgui.WindowFlagsNoMove*/ |imgui.WindowFlagsNoBackground|imgui.WindowFlagsNoScrollWithMouse|imgui.WindowFlagsNoBringToFrontOnFocus)

	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, vec2(5, 5))

	if l.mapsLoaded {
		l.drawMain()
	} else {
		l.drawSplash()
	}

	imgui.PopStyleVar()

	imgui.End()

	imgui.PopStyleVar()

	if lock {
		imgui.PopItemFlag()
	}

	DrawImgui()
}

func (l *launcher) drawMain() {
	w := imgui.WindowContentRegionMax().X

	imgui.PushFont(Font24)

	if imgui.BeginTableV("ltpanel", 2, imgui.TableFlagsSizingStretchProp, vec2(float32(w)/2, 0), -1) {
		imgui.TableSetupColumnV("ltpanel1", imgui.TableColumnFlagsWidthFixed, 0, uint(0))
		imgui.TableSetupColumnV("ltpanel2", imgui.TableColumnFlagsWidthStretch, 0, uint(1))

		imgui.TableNextColumn()

		imgui.AlignTextToFramePadding()
		imgui.Text("Mode:")

		imgui.TableNextColumn()

		imgui.SetNextItemWidth(-1)

		if imgui.BeginCombo("##mode", l.bld.currentMode.String()) {
			for _, m := range modes {
				if imgui.SelectableV(m.String(), l.bld.currentMode == m, 0, vzero()) {
					if m == Play {
						l.bld.currentPMode = Watch
					}

					if m != Replay {
						l.bld.replayPath = ""
						l.bld.currentReplay = nil
					}

					if m != NewKnockout {
						l.bld.knockoutReplays = nil
					}

					l.bld.currentMode = m
				}
			}

			imgui.EndCombo()
		}

		imgui.EndTable()
	}

	l.drawConfigPanel()

	l.drawControls()

	l.drawLowerPanel()

	if l.selectWindow != nil {
		l.selectWindow.update()
	}

	for i := 0; i < len(l.popupStack); i++ {
		p := l.popupStack[i]
		p.draw()
		if p.shouldClose() {
			l.popupStack = append(l.popupStack[:i], l.popupStack[i+1:]...)
			i--
		}
	}

	imgui.PopFont()

	if imgui.IsMouseClicked(0) && !l.danserRunning {
		l.showProgressBar = false
		l.recordStatus = ""
		l.recordProgress = 0
	}
}

func (l *launcher) drawSplash() {
	w, h := imgui.WindowContentRegionMax().X, imgui.WindowContentRegionMax().Y

	imgui.PushFont(Font48)

	splText := strings.Split(l.splashText, "\n")

	var height float32

	for _, sText := range splText {
		height += imgui.CalcTextSize(sText, false, 0).Y
	}

	var dHeight float32

	for _, sText := range splText {
		tSize := imgui.CalcTextSize(sText, false, 0)

		imgui.SetCursorPos(vec2(20+(w-tSize.X)/2, 20+(h-height)/2+dHeight))

		dHeight += tSize.Y

		imgui.Text(sText)
	}

	imgui.PopFont()
}

func (l *launcher) drawControls() {
	imgui.SetCursorPos(vec2(20, 88))
	switch l.bld.currentMode {
	case Replay:
		l.selectReplay()
	case NewKnockout:
		l.newKnockout()
	default:
		l.showSelect()
	}

	imgui.SetCursorPos(vec2(20, 204+34))

	w := imgui.WindowContentRegionMax().X

	if imgui.BeginTableV("abtn", 2, imgui.TableFlagsSizingStretchSame, vec2(float32(w)/2, -1), -1) {
		imgui.TableNextColumn()

		if imgui.ButtonV("Speed/Pitch", vec2(-1, imgui.TextLineHeight()*2)) {
			l.openPopup(newPopupF("Speed adjust", popMedium, func() {
				drawSpeedMenu(l.bld)
			}))
		}

		imgui.TableNextColumn()

		if l.bld.currentMode != Replay {
			if imgui.ButtonV("Mods", vec2(-1, imgui.TextLineHeight()*2)) {
				l.openPopup(newModPopup(l.bld))
			}
		}

		imgui.TableNextColumn()
		if l.bld.currentMode != Replay {
			if imgui.ButtonV("Adjust difficulty", vec2(-1, imgui.TextLineHeight()*2)) {
				l.openPopup(newPopupF("Difficulty adjust", popMedium, func() {
					drawParamMenu(l.bld)
				}))
			}
		} else {
			imgui.Dummy(vec2(-1, imgui.TextLineHeight()*2))
		}

		imgui.TableNextColumn()

		if l.bld.currentMode == CursorDance {
			if imgui.ButtonV("Mirrors/Tags", vec2(-1, imgui.TextLineHeight()*2)) {
				l.openPopup(newPopupF("Difficulty adjust", popDynamic, func() {
					drawCDMenu(l.bld)
				}))
			}
		}

		imgui.TableNextColumn()

		if imgui.ButtonV("Time/Offset", vec2(-1, imgui.TextLineHeight()*2)) {
			timePopup := newPopupF("Set times", popMedium, func() {
				drawTimeMenu(l.bld)
			})

			timePopup.setCloseListener(func() {
				if l.bld.currentMap != nil && l.bld.currentMap.LocalOffset != int(l.bld.offset.value) {
					l.bld.currentMap.LocalOffset = int(l.bld.offset.value)
					database.UpdateLocalOffset(l.bld.currentMap)
				}
			})

			l.openPopup(timePopup)
		}

		imgui.EndTable()
	}

}

func (l *launcher) selectReplay() {
	bSize := vec2((imgui.WindowWidth()-40)/4, imgui.TextLineHeight()*2)

	imgui.PushFont(Font32)

	if imgui.ButtonV("Select replay", bSize) {
		p, err := dialog.File().Filter("osu! replay file (*.osr)", "osr").Title("Select replay file").SetStartDir(l.currentConfig.General.GetReplaysDir()).Load()
		if err == nil {
			l.trySelectReplayFromPath(p)
		}
	}

	imgui.PopFont()

	imgui.PushFont(Font20)
	imgui.IndentV(5)

	if l.bld.currentReplay != nil {
		b := l.bld.currentMap

		mString := fmt.Sprintf("%s - %s [%s]\nPlayed by: %s", b.Artist, b.Name, b.Difficulty, l.bld.currentReplay.Username)

		imgui.PushTextWrapPosV(imgui.ContentRegionMax().X / 2)
		imgui.Text(mString)
		imgui.PopTextWrapPos()
	} else {
		imgui.Text("No replay selected")
	}

	imgui.UnindentV(5)
	imgui.PopFont()
}

func (l *launcher) trySelectReplayFromPath(p string) {
	replay, err := l.loadReplay(p)

	if err != nil {
		e := []rune(err.Error())
		showMessage(mError, string(unicode.ToUpper(e[0]))+string(e[1:]))
		return
	}

	l.trySelectReplay(replay)
}

func (l *launcher) trySelectReplaysFromPaths(p []string) {
	var errorCollection string
	var replays []*knockoutReplay

	for _, rPath := range p {
		replay, err := l.loadReplay(rPath)

		if err != nil {
			if errorCollection != "" {
				errorCollection += "\n"
			}

			errorCollection += fmt.Sprintf("%s:\n\t%s", filepath.Base(rPath), err)
		} else {
			replays = append(replays, replay)
		}
	}

	if errorCollection != "" {
		showMessage(mError, "There were errors opening replays:\n%s", errorCollection)
	}

	if replays != nil && len(replays) > 0 {
		found := false

		for _, replay := range replays {
			for _, bMap := range l.beatmaps {
				if strings.ToLower(bMap.MD5) == strings.ToLower(replay.parsedReplay.BeatmapMD5) {
					l.bld.currentMode = NewKnockout
					l.bld.setMap(bMap)

					found = true
					break
				}
			}

			if found {
				break
			}
		}

		if !found {
			showMessage(mError, "Replays use an unknown map. Please download the map beforehand.")
		} else {
			var finalReplays []*knockoutReplay

			for _, replay := range replays {
				if strings.ToLower(l.bld.currentMap.MD5) == strings.ToLower(replay.parsedReplay.BeatmapMD5) {
					finalReplays = append(finalReplays, replay)
				}
			}

			slices.SortFunc(finalReplays, func(a, b *knockoutReplay) bool {
				return a.parsedReplay.Score > b.parsedReplay.Score
			})

			l.bld.knockoutReplays = finalReplays
		}
	}
}

func (l *launcher) trySelectReplay(replay *knockoutReplay) {
	for _, bMap := range l.beatmaps {
		if strings.ToLower(bMap.MD5) == strings.ToLower(replay.parsedReplay.BeatmapMD5) {
			l.bld.currentMode = Replay
			l.bld.replayPath = replay.path
			l.bld.currentReplay = replay.parsedReplay
			l.bld.setMap(bMap)

			return
		}
	}

	showMessage(mError, "Replay uses an unknown map. Please download the map beforehand.")
}

func (l *launcher) newKnockout() {
	bSize := vec2((imgui.WindowWidth()-40)/4, imgui.TextLineHeight()*2)

	imgui.PushFont(Font32)

	if imgui.ButtonV("Select replays", bSize) {
		kPath := getAbsPath(launcherConfig.LastKnockoutPath)

		_, err := os.Lstat(kPath)
		if err != nil {
			kPath = env.DataDir()
		}

		p, err := dialog.File().Filter("osu! replay file (*.osr)", "osr").Title("Select replay files").SetStartDir(kPath).LoadMultiple()
		if err == nil {
			launcherConfig.LastKnockoutPath = getRelativeOrABSPath(filepath.Dir(p[0]))
			saveLauncherConfig()

			l.trySelectReplaysFromPaths(p)
		}
	}

	imgui.PopFont()

	imgui.PushFont(Font20)

	imgui.IndentV(5)

	if l.bld.knockoutReplays != nil && l.bld.currentMap != nil {
		b := l.bld.currentMap

		imgui.PushTextWrapPosV(imgui.ContentRegionMax().X / 2)

		imgui.Text(fmt.Sprintf("%s - %s [%s]", b.Artist, b.Name, b.Difficulty))

		imgui.AlignTextToFramePadding()

		imgui.Text(fmt.Sprintf("%d replays loaded", len(l.bld.knockoutReplays)))

		imgui.PopTextWrapPos()

		imgui.SameLine()

		if imgui.Button("Manage##knockout") {
			l.openPopup(newPopupF("Manage replays", popBig, func() {
				drawReplayManager(l.bld)
			}))
		}
	} else {
		imgui.Text("No replays selected")
	}

	imgui.UnindentV(5)

	imgui.PopFont()
}

func (l *launcher) loadReplay(p string) (*knockoutReplay, error) {
	if !strings.HasSuffix(p, ".osr") {
		return nil, fmt.Errorf("it's not a replay file")
	}

	rData, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %s", err)
	}

	replay, err := rplpa.ParseReplay(rData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse replay: %s", err)
	}

	if replay.ReplayData == nil || len(replay.ReplayData) < 2 {
		return nil, errors.New("replay is missing input data")
	}

	// dump unneeded data as it's not needed anymore to save memory
	replay.LifebarGraph = nil
	replay.ReplayData = nil

	return &knockoutReplay{
		path:         p,
		parsedReplay: replay,
		included:     true,
	}, nil
}

func (l *launcher) showSelect() {
	bSize := vec2((imgui.WindowWidth()-40)/4, imgui.TextLineHeight()*2)

	imgui.PushFont(Font32)

	if imgui.ButtonV("Select map", bSize) {
		if l.selectWindow == nil {
			l.selectWindow = newSongSelectPopup(l.bld, l.beatmaps)
		}

		l.selectWindow.open()
		l.openPopup(l.selectWindow)
	}

	imgui.PopFont()

	imgui.PushFont(Font20)

	imgui.IndentV(5)

	if l.bld.currentMap != nil {
		b := l.bld.currentMap

		mString := fmt.Sprintf("%s - %s [%s]", b.Artist, b.Name, b.Difficulty)

		imgui.PushTextWrapPosV(imgui.ContentRegionMax().X / 2)
		imgui.Text(mString)
		imgui.PopTextWrapPos()
	} else {
		imgui.Text("No map selected")
	}

	imgui.UnindentV(5)

	imgui.PopFont()
}

func (l *launcher) drawLowerPanel() {
	w, h := imgui.WindowContentRegionMax().X, imgui.WindowContentRegionMax().Y

	if l.bld.currentMode != Play {
		showProgress := l.bld.currentPMode == Record && l.showProgressBar

		spacing := imgui.FrameHeightWithSpacing()
		if showProgress {
			spacing *= 2
		}

		imgui.SetCursorPos(vec2(20, h-spacing))

		imgui.SetNextItemWidth((imgui.WindowWidth() - 40) / 4)

		if imgui.BeginCombo("##Watch mode", l.bld.currentPMode.String()) {
			for _, m := range pModes {
				if imgui.SelectableV(m.String(), l.bld.currentPMode == m, 0, vzero()) {
					l.bld.currentPMode = m
				}
			}

			imgui.EndCombo()
		}

		if l.bld.currentPMode != Watch {
			imgui.SameLine()
			if imgui.Button("Configure") {
				l.openPopup(newPopupF("Record settings", popDynamic, func() {
					drawRecordMenu(l.bld)
				}))
			}
		}

		imgui.SetCursorPos(vec2(imgui.WindowContentRegionMin().X, h-imgui.FrameHeightWithSpacing()))

		if showProgress {
			if strings.HasPrefix(l.recordStatus, "Done") {
				imgui.PushStyleColor(imgui.StyleColorPlotHistogram, imgui.Vec4{
					X: 0.16,
					Y: 0.75,
					Z: 0.18,
					W: 1,
				})
			} else {
				imgui.PushStyleColor(imgui.StyleColorPlotHistogram, imgui.CurrentStyle().Color(imgui.StyleColorCheckMark))
			}

			imgui.ProgressBarV(l.recordProgress, vec2(w/2, imgui.FrameHeight()), l.recordStatus)

			if l.encodeInProgress {
				imgui.PushFont(Font16)

				cPos := imgui.CursorPos()

				imgui.Text(l.recordStatusSpeed)

				cPos.X += 95

				imgui.SetCursorPos(cPos)

				eta := int(time.Since(l.encodeStart).Seconds())

				imgui.Text("| Elapsed: " + util.FormatSeconds(eta))

				cPos.X += 135

				imgui.SetCursorPos(cPos)

				imgui.Text("| " + l.recordStatusETA)

				imgui.PopFont()
			}

			imgui.PopStyleColor()
		}
	}

	fHwS := imgui.FrameHeightWithSpacing()*2 - imgui.CurrentStyle().FramePadding().X

	bW := (w) / 4

	imgui.SetCursorPos(vec2(imgui.WindowContentRegionMax().X-w/2.5, h-imgui.FrameHeightWithSpacing()*2))

	centerTable("dansebutton", w/2.5, func() {
		imgui.PushFont(Font48)
		{
			dRun := l.danserRunning && l.bld.currentPMode == Record

			s := (l.bld.currentMode == Replay && l.bld.currentReplay == nil) ||
				(l.bld.currentMode != Replay && l.bld.currentMap == nil) ||
				(l.bld.currentMode == NewKnockout && l.bld.numKnockoutReplays() == 0)

			if !dRun {
				if s {
					imgui.PushItemFlag(imgui.ItemFlagsDisabled, true)
				}
			} else {
				imgui.PopItemFlag()
			}

			name := "danse!"
			if dRun {
				name = "CANCEL"
			}

			if imgui.ButtonV(name, vec2(bW, fHwS)) {
				if dRun {
					if l.danserCmd != nil {
						goroutines.Run(func() {
							res := showMessage(mQuestion, "Do you really want to cancel?")

							if res && l.danserCmd != nil {
								l.danserCmd.Process.Kill()
								l.danserCleanup(false)
							}
						})
					}
				} else {
					if l.selectWindow != nil {
						l.selectWindow.stopPreview()
					}

					log.Println(l.bld.getArguments())

					l.triangleSpeed.AddEventS(l.triangleSpeed.GetTime(), l.triangleSpeed.GetTime()+1000, 50, 1)

					if l.bld.currentPMode != Watch {
						l.startDanser()
					} else {
						goroutines.Run(func() {
							time.Sleep(500 * time.Millisecond)
							l.startDanser()
						})
					}
				}
			}

			if !dRun {
				if s {
					imgui.PopItemFlag()
				}
			} else {
				imgui.PushItemFlag(imgui.ItemFlagsDisabled, true)
			}

			imgui.PopFont()
		}
	})
}

func (l *launcher) drawConfigPanel() {
	w := imgui.WindowContentRegionMax().X

	imgui.SetCursorPos(vec2(imgui.WindowContentRegionMax().X-float32(w)/2.5, 20))

	if imgui.BeginTableV("rtpanel", 2, imgui.TableFlagsSizingStretchProp, vec2(float32(w)/2.5, 0), -1) {
		imgui.TableSetupColumnV("rtpanel1", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("rtpanel2", imgui.TableColumnFlagsWidthFixed, 0, 1)

		imgui.TableNextColumn()

		if imgui.ButtonV("Launcher settings", vec2(-1, 0)) {
			lEditor := newPopupF("About", popMedium, drawLauncherConfig)

			lEditor.setCloseListener(func() {
				saveLauncherConfig()
			})

			l.openPopup(lEditor)
		}

		imgui.TableNextColumn()

		if imgui.Button("About") {
			l.openPopup(newPopupF("About", popDynamic, func() {
				drawAbout(l.coin.Texture.Texture)
			}))
		}

		imgui.TableNextColumn()

		imgui.AlignTextToFramePadding()
		imgui.Text("Config:")

		imgui.SameLine()

		imgui.SetNextItemWidth(-1)

		mWidth := imgui.CalcItemWidth() - imgui.CurrentStyle().FramePadding().X*2

		if imgui.BeginComboV("##config", l.bld.config, imgui.ComboFlagsHeightLarge) {
			for _, s := range l.configList {
				mWidth = mutils.Max(mWidth, imgui.CalcTextSize(s, false, 0).X+20)
			}

			imgui.SetNextItemWidth(mWidth)

			focusScroll := searchBox("##configSearch", &l.configSearch)

			if !imgui.IsMouseClicked(0) && !imgui.IsMouseClicked(1) && !imgui.IsAnyItemActive() && !(imgui.IsWindowFocusedV(imgui.FocusedFlagsChildWindows) && !imgui.IsWindowFocused()) {
				imgui.SetKeyboardFocusHereV(-1)
			}

			if imgui.Selectable("Create new...") {
				l.newCloneOpened = true
				l.configManiMode = New
			}

			imgui.PushStyleVarFloat(imgui.StyleVarFrameRounding, 0)
			imgui.PushStyleVarFloat(imgui.StyleVarFrameBorderSize, 0)
			imgui.PushStyleVarVec2(imgui.StyleVarFramePadding, vzero())
			imgui.PushStyleColor(imgui.StyleColorFrameBg, imgui.Vec4{X: 0, Y: 0, Z: 0, W: 0})

			searchResults := make([]string, 0, len(l.configList))

			search := strings.ToLower(l.configSearch)

			for _, s := range l.configList {
				if l.configSearch == "" || strings.Contains(strings.ToLower(s), search) {
					searchResults = append(searchResults, s)
				}
			}

			if len(searchResults) > 0 {
				sHeight := float32(mutils.Min(8, len(searchResults)))*imgui.FrameHeightWithSpacing() - imgui.CurrentStyle().ItemSpacing().Y/2

				if imgui.BeginListBoxV("##blistbox", vec2(mWidth, sHeight)) {
					focusScroll = focusScroll || imgui.IsWindowAppearing()

					for _, s := range searchResults {
						if selectableFocus(s, s == l.bld.config, focusScroll) {
							if s != l.bld.config {
								l.setConfig(s)
							}
						}

						if imgui.IsMouseClicked(1) && imgui.IsItemHovered() {
							l.configEditOpened = true

							imgui.SetNextWindowPosV(imgui.MousePos(), imgui.ConditionAlways, vzero())

							imgui.OpenPopup("##context" + s)
						}

						if imgui.BeginPopupModalV("##context"+s, &l.configEditOpened, imgui.WindowFlagsNoCollapse|imgui.WindowFlagsNoResize|imgui.WindowFlagsAlwaysAutoResize|imgui.WindowFlagsNoMove|imgui.WindowFlagsNoTitleBar) {
							if s != "default" {
								if imgui.Selectable("Rename") {
									l.newCloneOpened = true
									l.configPrevName = s
									l.configManiMode = Rename
								}
							}

							if imgui.Selectable("Clone") {
								l.newCloneOpened = true
								l.configPrevName = s
								l.configManiMode = Clone
							}

							if s != "default" {
								if imgui.Selectable("Remove") {
									if showMessage(mQuestion, "Are you sure you want to remove \"%s\" profile?", s) {
										l.removeConfig(s)
									}
								}
							}

							if (imgui.IsMouseClicked(0) || imgui.IsMouseClicked(1)) && !imgui.IsWindowHoveredV(imgui.HoveredFlagsRootAndChildWindows|imgui.HoveredFlagsAllowWhenBlockedByActiveItem|imgui.HoveredFlagsAllowWhenBlockedByPopup) {
								l.configEditOpened = false
							}

							imgui.EndPopup()
						}
					}

					imgui.EndListBox()
				}
			}

			imgui.PopStyleVar()
			imgui.PopStyleVar()
			imgui.PopStyleVar()
			imgui.PopStyleColor()

			imgui.EndCombo()
		}

		imgui.TableNextColumn()

		if imgui.ButtonV("Edit", vec2(-1, 0)) {
			sEditor := newSettingsEditor(l.currentConfig)

			sEditor.setCloseListener(func() {
				settings.SaveCredentials(false)
				l.currentConfig.Save("", false)

				if !compareDirs(l.currentConfig.General.OsuSongsDir, settings.General.OsuSongsDir) {
					showMessage(mInfo, "This config has different osu! Songs directory.\nRestart the launcher to see updated maps")
				}
			})

			l.openPopup(sEditor)
		}

		imgui.EndTable()
	}

	if l.newCloneOpened {
		popupSmall("Clone/new box", &l.newCloneOpened, true, func() {
			if imgui.BeginTable("rfa", 1) {
				imgui.TableNextColumn()

				imgui.Text("Name:")

				imgui.SameLine()

				imgui.SetNextItemWidth(imgui.TextLineHeight() * 10)

				if imgui.InputTextV("##nclonename", &l.newCloneName, imgui.InputTextFlagsCallbackCharFilter, imguiPathFilter) {
					l.newCloneName = strings.TrimSpace(l.newCloneName)
				}

				if !imgui.IsAnyItemActive() && !imgui.IsMouseClicked(0) {
					imgui.SetKeyboardFocusHereV(-1)
				}

				imgui.TableNextColumn()

				cPos := imgui.CursorPos()

				imgui.SetCursorPos(vec2(cPos.X+(imgui.ContentRegionAvail().X-imgui.CalcTextSize("Save", false, 0).X-imgui.CurrentStyle().FramePadding().X*2)/2, cPos.Y))

				e := l.newCloneName == ""

				if e {
					imgui.PushItemFlag(imgui.ItemFlagsDisabled, true)
				}

				if imgui.Button("Save##newclone") {
					_, err := os.Stat(filepath.Join(env.ConfigDir(), l.newCloneName+".json"))
					if err == nil {
						showMessage(mError, "Config with that name already exists!\nPlease pick a different name")
					} else {
						log.Println("ok")
						switch l.configManiMode {
						case Rename:
							l.renameConfig(l.configPrevName, l.newCloneName)
						case Clone:
							l.cloneConfig(l.configPrevName, l.newCloneName)
						case New:
							l.createConfig(l.newCloneName)
						}

						l.newCloneOpened = false
						l.newCloneName = ""
					}
				}

				if e {
					imgui.PopItemFlag()
				}

				imgui.EndTable()
			}
		})
	}
}

func (l *launcher) tryCreateDefaultConfig() {
	_, err := os.Stat(filepath.Join(env.ConfigDir(), "default.json"))
	if err != nil {
		l.createConfig("default")
	}
}

func (l *launcher) createConfig(name string) {
	vm := glfw.GetPrimaryMonitor().GetVideoMode()

	conf := settings.NewConfigFile()
	conf.Graphics.SetDefaults(int64(vm.Width), int64(vm.Height))
	conf.Save(filepath.Join(env.ConfigDir(), name+".json"), true)

	l.createConfigList()

	l.setConfig(name)
}

func (l *launcher) removeConfig(name string) {
	os.Remove(filepath.Join(env.ConfigDir(), name+".json"))

	l.createConfigList()

	if l.bld.config == name {
		l.setConfig("default")
	}
}

func (l *launcher) cloneConfig(toClone, name string) {
	cConfig, err := l.loadConfig(toClone)

	if err != nil {
		showMessage(mError, err.Error())
		return
	}

	cConfig.Save(filepath.Join(env.ConfigDir(), name+".json"), true)

	l.createConfigList()

	l.setConfig(name)
}

func (l *launcher) renameConfig(toRename, name string) {
	cConfig, err := l.loadConfig(toRename)

	if err != nil {
		showMessage(mError, err.Error())
		return
	}

	cConfig.Save(filepath.Join(env.ConfigDir(), name+".json"), true)

	os.Remove(filepath.Join(env.ConfigDir(), toRename+".json"))

	l.createConfigList()

	l.setConfig(name)
}

func (l *launcher) createConfigList() {
	l.configList = []string{}

	filepath.Walk(env.ConfigDir(), func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			stPath := strings.ReplaceAll(strings.TrimPrefix(strings.TrimSuffix(path, ".json"), env.ConfigDir()+string(os.PathSeparator)), "\\", "/")

			if stPath != "credentials" && stPath != "default" && stPath != "launcher" {
				log.Println("Config:", stPath)
				l.configList = append(l.configList, stPath)
			}
		}

		return nil
	})

	sort.Strings(l.configList)

	l.configList = append([]string{"default"}, l.configList...)
}

func (l *launcher) loadConfig(name string) (*settings.Config, error) {
	f, err := os.Open(filepath.Join(env.ConfigDir(), name+".json"))
	if err != nil {
		return nil, fmt.Errorf("invalid file state. Please don't modify the folder while launcher is running. Error: %s", err)
	}

	defer f.Close()

	return settings.LoadConfig(f)
}

func (l *launcher) setConfig(s string) {
	eConfig, err := l.loadConfig(s)

	if err != nil {
		showMessage(mError, "Failed to read \"%s\" profile. Error: %s", s, err)
	} else {
		if !compareDirs(eConfig.General.OsuSongsDir, settings.General.OsuSongsDir) {
			showMessage(mInfo, "This config has different osu! Songs directory.\nRestart the launcher to see updated maps")
		}

		l.bld.config = s
		l.currentConfig = eConfig

		*launcherConfig.Profile = l.bld.config
		saveLauncherConfig()
	}
}

func (l *launcher) startDanser() {
	l.recordProgress = 0
	l.recordStatus = ""
	l.recordStatusSpeed = ""
	l.recordStatusETA = ""
	l.encodeInProgress = false

	dExec := os.Args[0]

	if build.Stream == "Release" {
		dExec = filepath.Join(env.LibDir(), "danser")
	}

	l.danserCmd = exec.Command(dExec, l.bld.getArguments()...)

	rFile, oFile, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	l.danserCmd.Stderr = os.Stderr
	l.danserCmd.Stdin = os.Stdin
	l.danserCmd.Stdout = io.MultiWriter(os.Stdout, oFile)

	err = l.danserCmd.Start()
	if err != nil {
		showMessage(mError, "danser failed to start! %s", err.Error())
		return
	}

	if l.bld.currentPMode == Watch {
		l.win.Iconify()
	} else if l.bld.currentPMode == Record {
		l.showProgressBar = true
	}

	l.danserRunning = true
	l.recordStatus = "Preparing..."

	panicMessage := ""
	panicWait := &sync.WaitGroup{}
	panicWait.Add(1)

	resultFile := ""

	goroutines.Run(func() {
		sc := bufio.NewScanner(rFile)

		l.encodeInProgress = false

		for sc.Scan() {
			line := sc.Text()

			if strings.Contains(line, "panic:") {
				panicMessage = line[strings.Index(line, "panic:"):]
				panicWait.Done()
			}

			if strings.Contains(line, "Starting encoding!") {
				l.encodeInProgress = true
				l.encodeStart = time.Now()
			}

			if strings.Contains(line, "Finishing rendering") {
				l.encodeInProgress = false

				l.recordProgress = 1
				l.recordStatus = "Finalizing..."
				l.recordStatusSpeed = ""
				l.recordStatusETA = ""
			}

			if idx := strings.Index(line, "Video is available at: "); idx > -1 {
				resultFile = strings.TrimPrefix(line[idx:], "Video is available at: ")
			}

			if idx := strings.Index(line, "Screenshot "); idx > -1 && strings.Contains(line, " saved!") {
				resultFile = strings.TrimSuffix(strings.TrimPrefix(line[idx:], "Screenshot "), " saved!")
				resultFile = filepath.Join(env.DataDir(), "screenshots", resultFile)
			}

			if strings.Contains(line, "Progress") && l.encodeInProgress {
				line = line[strings.Index(line, "Progress"):]

				rStats := strings.Split(line, ",")

				spl := strings.TrimSpace(strings.Split(rStats[0], ":")[1])

				l.recordStatus = spl

				l.recordStatusSpeed = strings.TrimSpace(rStats[1])
				l.recordStatusETA = strings.TrimSpace(rStats[2])

				speed := strings.TrimSpace(strings.Split(rStats[1], ":")[1])

				speedP, _ := strconv.ParseFloat(speed[:len(speed)-1], 32)

				l.triangleSpeed.AddEvent(l.triangleSpeed.GetTime(), l.triangleSpeed.GetTime()+500, speedP)

				at, _ := strconv.Atoi(spl[:len(spl)-1])

				l.recordProgress = float32(at) / 100
			}
		}

		l.recordProgress = 100
		l.recordStatus = "Done in " + util.FormatSeconds(int(time.Since(l.encodeStart).Seconds()))
		l.recordStatusSpeed = ""
		l.recordStatusETA = ""
	})

	goroutines.Run(func() {
		err = l.danserCmd.Wait()

		l.danserCleanup(err == nil)

		if err != nil {
			panicWait.Wait()

			mainthread.Call(func() {
				pMsg := panicMessage
				if idx := strings.Index(pMsg, "Error:"); idx > -1 {
					pMsg = pMsg[:idx-1] + "\n\n" + pMsg[idx+7:]
				}

				showMessage(mError, "danser crashed! %s\n\n%s", err.Error(), pMsg)
			})
		} else if l.bld.currentPMode != Watch && l.bld.currentMode != Play {
			if launcherConfig.ShowFileAfter && resultFile != "" {
				platform.ShowFileInManager(resultFile)
			}

			C.beep_custom()
		}

		rFile.Close()
		oFile.Close()

		l.win.Restore()
	})
}

func (l *launcher) danserCleanup(success bool) {
	l.recordStatusSpeed = ""
	l.recordStatusETA = ""
	l.danserRunning = false
	l.triangleSpeed.AddEvent(l.triangleSpeed.GetTime(), l.triangleSpeed.GetTime()+500, 1)
	l.danserCmd = nil

	if !success {
		l.recordStatus = ""
		l.showProgressBar = false
	}
}

func (l *launcher) openPopup(p iPopup) {
	l.popupStack = append(l.popupStack, p)
}
