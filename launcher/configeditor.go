package launcher

import (
	"fmt"
	"github.com/inkyblackness/imgui-go/v4"
	"github.com/sqweek/dialog"
	"github.com/wieku/danser-go/app/settings"
	"github.com/wieku/danser-go/framework/env"
	"github.com/wieku/danser-go/framework/math/color"
	"github.com/wieku/danser-go/framework/math/math32"
	"github.com/wieku/danser-go/framework/math/mutils"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const padY = 30

var nameSplitter = regexp.MustCompile(`[A-Z]+[^A-Z]*`)

type settingsEditor struct {
	*popup

	searchCache  map[string]int
	scrollTo     string
	blockSearch  bool
	searchString string

	current  *settings.Config
	combined *settings.CombinedConfig

	listenerCalled bool
	sectionCache   map[string]imgui.Vec2

	active      string
	lastActive  string
	pwShowHide  map[string]bool
	comboSearch map[string]string
}

func newSettingsEditor(config *settings.Config) *settingsEditor {
	editor := &settingsEditor{
		popup:        newPopup("Settings Editor", popBig),
		searchCache:  make(map[string]int),
		sectionCache: make(map[string]imgui.Vec2),
		pwShowHide:   make(map[string]bool),
		comboSearch:  make(map[string]string),
	}

	editor.internalDraw = editor.drawEditor

	editor.current = config
	editor.combined = config.GetCombined()

	editor.search()

	return editor
}

func (editor *settingsEditor) drawEditor() {
	settings.General.OsuSkinsDir = editor.combined.General.OsuSkinsDir

	imgui.PushStyleColor(imgui.StyleColorWindowBg, vec4(0, 0, 0, .9))
	imgui.PushStyleColor(imgui.StyleColorFrameBg, vec4(.2, .2, .2, 1))

	imgui.PushStyleVarVec2(imgui.StyleVarCellPadding, vec2(2, 0))

	if imgui.BeginTableV("Edit main table", 2, imgui.TableFlagsSizingStretchProp, vec2(-1, -1), -1) {
		imgui.PopStyleVar()

		imgui.TableSetupColumnV("Edit main table 1", imgui.TableColumnFlagsWidthFixed, 0, uint(0))
		imgui.TableSetupColumnV("Edit main table 2", imgui.TableColumnFlagsWidthStretch, 0, uint(1))

		imgui.TableNextColumn()

		imgui.PushStyleColor(imgui.StyleColorChildBg, vec4(0, 0, 0, .5))

		imgui.PushFont(FontAw)
		{

			imgui.PushStyleVarFloat(imgui.StyleVarScrollbarSize, 9)

			if imgui.BeginChildV("##Editor navigation", vec2(imgui.FontSize()*1.5+9, -1), false, imgui.WindowFlagsAlwaysVerticalScrollbar) {
				editor.scrollTo = ""

				imgui.PushStyleVarFloat(imgui.StyleVarFrameRounding, 0)
				imgui.PushStyleVarFloat(imgui.StyleVarFrameBorderSize, 0)
				imgui.PushStyleVarVec2(imgui.StyleVarItemSpacing, vzero())

				editor.buildNavigationFor(editor.combined)

				imgui.PopStyleVar()
				imgui.PopStyleVar()
				imgui.PopStyleVar()
			}

			imgui.PopStyleVar()

			imgui.EndChild()
		}
		imgui.PopFont()

		imgui.PopStyleColor()

		imgui.TableNextColumn()

		imgui.PushFont(Font32)
		{
			imgui.SetNextItemWidth(-1)

			if searchBox("##Editor search", &editor.searchString) {
				editor.search()
			}

			if !editor.blockSearch && !imgui.IsAnyItemActive() && !imgui.IsMouseClicked(0) {
				imgui.SetKeyboardFocusHereV(-1)
			}
		}
		imgui.PopFont()

		imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, vec2(5, 0))

		if imgui.BeginChildV("##Editor main", vec2(-1, -1), false, imgui.WindowFlagsAlwaysUseWindowPadding) {
			imgui.PopStyleVar()

			editor.blockSearch = false

			imgui.PushFont(Font20)

			editor.drawSettings()

			imgui.PopFont()
		} else {
			imgui.PopStyleVar()
		}

		imgui.EndChild()

		imgui.EndTable()
	} else {
		imgui.PopStyleVar()
	}

	imgui.PopStyleColor()
	imgui.PopStyleColor()
}

func (editor *settingsEditor) search() {
	editor.sectionCache = make(map[string]imgui.Vec2)
	editor.searchCache = make(map[string]int)
	editor.buildSearchCache("Main", reflect.ValueOf(editor.combined), editor.searchString, false)
}

func (editor *settingsEditor) buildSearchCache(path string, u reflect.Value, search string, omitSearch bool) bool {
	typ := u.Elem()
	def := u.Type().Elem()

	count := typ.NumField()

	found := false

	skipMap := make(map[string]uint8)
	consumed := make(map[string]uint8)

	for i := 0; i < count; i++ {
		field := typ.Field(i)
		dF := def.Field(i)

		if def.Field(i).Tag.Get("skip") != "" {
			continue
		}

		if editor.shouldBeHidden(consumed, skipMap, typ, dF) {
			continue
		}

		label := editor.getLabel(dF)

		sPath := path + "." + label

		match := omitSearch || strings.Contains(strings.ToLower(label), search)

		if field.Type().Kind() == reflect.Ptr && (field.CanInterface() || def.Field(i).Anonymous) && !field.IsNil() && !field.Type().AssignableTo(reflect.TypeOf(&settings.HSV{})) {
			sub := editor.buildSearchCache(sPath, field, search, match)
			match = match || sub
		} else if field.Type().Kind() == reflect.Slice && field.CanInterface() {
			for j := 0; j < field.Len(); j++ {
				sub := editor.buildSearchCache(sPath, field.Index(j), search, match)
				match = match || sub
			}
		}

		if match {
			editor.searchCache[sPath] = 1
			found = true
		}
	}

	return found
}

func (editor *settingsEditor) buildNavigationFor(u interface{}) {
	typ := reflect.ValueOf(u).Elem()
	def := reflect.TypeOf(u).Elem()

	count := typ.NumField()

	imgui.PushStyleColor(imgui.StyleColorButton, vec4(0, 0, 0, 0))

	buttonSize := imgui.FontSize() * 1.5

	cAvail := imgui.ContentRegionAvail().Y
	sc1 := imgui.ScrollY()
	sc2 := sc1 + cAvail

	for i := 0; i < count; i++ {
		label := editor.getLabel(def.Field(i))

		if editor.searchCache["Main."+label] > 0 && (typ.Field(i).CanInterface() && !typ.Field(i).IsNil()) {
			if editor.active == label {
				cColor := imgui.CurrentStyle().Color(imgui.StyleColorCheckMark)

				imgui.PushStyleColor(imgui.StyleColorButton, vec4(0.2, 0.2, 0.2, 0.6))
				imgui.PushStyleColor(imgui.StyleColorText, vec4(cColor.X*1.2, cColor.Y*1.2, cColor.Z*1.2, 1))
			}

			c1 := imgui.CursorPos().Y

			if imgui.ButtonV(def.Field(i).Tag.Get("icon"), vec2(buttonSize, buttonSize)) {
				editor.scrollTo = label
			}

			c2 := imgui.CursorPos().Y

			if editor.active == label {
				if editor.lastActive != editor.active {
					if c2 > sc2 {
						imgui.SetScrollY(c2 - cAvail)
					}

					if c1 < sc1 {
						imgui.SetScrollY(c1)
					}

					editor.lastActive = editor.active
				}

				imgui.PopStyleColor()
				imgui.PopStyleColor()
			}

			if imgui.IsItemHovered() {
				imgui.PushFont(Font24)
				imgui.BeginTooltip()
				imgui.SetTooltip(label)
				imgui.EndTooltip()
				imgui.PopFont()
			}
		}
	}

	imgui.PopStyleColor()
}

func (editor *settingsEditor) drawSettings() {
	rVal := reflect.ValueOf(editor.combined)

	typ := rVal.Elem()
	def := rVal.Type().Elem()

	count := typ.NumField()

	sc1 := imgui.ScrollY()
	sc2 := sc1 + imgui.ContentRegionAvail().Y

	forceDrawNew := false

	for i, j := 0, 0; i < count; i++ {
		field := typ.Field(i)
		dF := def.Field(i)

		lbl := editor.getLabel(dF)

		if editor.searchCache["Main."+lbl] == 0 {
			continue
		}

		if field.CanInterface() && field.Type().Kind() == reflect.Ptr && !field.IsNil() {
			if j > 0 {
				imgui.Dummy(vec2(0, 2*padY))
			}

			drawNew := true
			if v, ok := editor.sectionCache["Main."+lbl]; ok {
				if editor.scrollTo == lbl {
					imgui.SetScrollY(v.X)
				}

				if (sc1 > v.Y || sc2 < v.X) && !forceDrawNew {
					drawNew = false

					imgui.SetCursorPos(vec2(imgui.CursorPosX(), v.Y))
				}
			}

			if drawNew {
				iSc1 := imgui.CursorPos().Y

				editor.buildMainSection("##"+dF.Name, "Main."+lbl, lbl, field)

				iSc2 := imgui.CursorPos().Y

				cCacheVal := editor.sectionCache["Main."+lbl]

				if math32.Abs(cCacheVal.X-iSc1) > 0.001 || math32.Abs(cCacheVal.Y-iSc2) > 0.001 { // if size of the section changed (dynamically hidden items/array changes) we need to redraw stuff below to have good metrics
					forceDrawNew = true
				}

				editor.sectionCache["Main."+lbl] = vec2(iSc1, iSc2)
			}

			j++
		}
	}
}

func (editor *settingsEditor) buildMainSection(jsonPath, sPath, name string, u reflect.Value) {
	posLocal := imgui.CursorPos()

	imgui.PushFont(Font48)
	imgui.Text(name)

	imgui.PopFont()
	imgui.Separator()

	editor.traverseChildren(jsonPath, sPath, u, reflect.StructField{})

	posLocal1 := imgui.CursorPos()

	scrY := imgui.ScrollY()
	if scrY >= posLocal.Y-padY*2 && scrY <= posLocal1.Y {
		editor.active = name
	}
}

func (editor *settingsEditor) subSectionTempl(sPath, name string, first, last bool, afterTitle, content func()) {
	if editor.searchCache[sPath] == 0 {
		return
	}

	if !first {
		imgui.Dummy(vec2(0, padY/2))
	}

	pos := imgui.CursorScreenPos()

	imgui.Dummy(vec2(3, 0))
	imgui.SameLine()

	imgui.BeginGroup()

	imgui.PushFont(Font24)
	imgui.Text(strings.ToUpper(name))

	afterTitle()

	imgui.PopFont()

	imgui.WindowDrawList().AddLine(imgui.CursorScreenPos(), imgui.CursorScreenPos().Plus(vec2(imgui.ContentRegionMax().X, 0)), imgui.PackedColorFromVec4(imgui.CurrentStyle().Color(imgui.StyleColorSeparator)))

	imgui.Spacing()

	content()

	imgui.EndGroup()

	pos1 := imgui.CursorScreenPos()

	pos1.X = pos.X

	imgui.WindowDrawList().AddLine(pos, pos1, imgui.PackedColorFromVec4(vec4(1.0, 1.0, 1.0, 1.0)))

	if !last {
		imgui.Dummy(vec2(0, padY/2))
	}
}

func (editor *settingsEditor) buildSubSection(jsonPath, sPath, name string, u reflect.Value, d reflect.StructField, first, last bool) {
	editor.subSectionTempl(sPath, name, first, last, func() {}, func() {
		editor.traverseChildren(jsonPath, sPath, u, d)
	})
}

func (editor *settingsEditor) buildArray(jsonPath, sPath, name string, u reflect.Value, d reflect.StructField, first, last bool) {
	editor.subSectionTempl(sPath, name, first, last, func() {
		imgui.SameLine()
		imgui.Dummy(vec2(2, 0))
		imgui.SameLine()

		ImIO.SetFontGlobalScale(20.0 / 32)
		imgui.PushFont(FontAw)

		if imgui.Button("+" + jsonPath) {
			if fName, ok := d.Tag.Lookup("new"); ok {
				u.Set(reflect.Append(u, reflect.ValueOf(settings.DefaultsFactory).MethodByName(fName).Call(nil)[0]))
			}
		}

		ImIO.SetFontGlobalScale(1)
		imgui.PopFont()
	}, func() {
		for j := 0; j < u.Len(); j++ {
			if editor.buildArrayElement(fmt.Sprintf("%s[%d]", jsonPath, j), sPath, u.Index(j), d, j) && u.Len() > 1 {
				u.Set(reflect.AppendSlice(u.Slice(0, j), u.Slice(j+1, u.Len())))
				j--
			}
		}
	})
}

func (editor *settingsEditor) buildArrayElement(jsonPath, sPath string, u reflect.Value, d reflect.StructField, childNum int) (removed bool) {
	if editor.searchCache[sPath] == 0 {
		return false
	}

	if childNum > 0 {
		imgui.Dummy(vec2(0, padY/3))
	}

	contentAvail := imgui.ContentRegionAvail().X

	if imgui.BeginTableV(jsonPath+"tae", 2, imgui.TableFlagsSizingStretchProp|imgui.TableFlagsNoPadInnerX|imgui.TableFlagsNoPadOuterX|imgui.TableFlagsNoClip, vec2(contentAvail, 0), contentAvail) {
		bWidth := imgui.FontSize() + imgui.CurrentStyle().FramePadding().X*2 + imgui.CurrentStyle().ItemSpacing().X*2 + 1

		imgui.TableSetupColumnV(jsonPath+"tae1", imgui.TableColumnFlagsWidthFixed, bWidth, uint(0))
		imgui.TableSetupColumnV(jsonPath+"tae2", imgui.TableColumnFlagsWidthFixed, contentAvail-bWidth, uint(1))

		imgui.TableNextColumn()
		imgui.TableNextColumn()

		pos := imgui.CursorScreenPos().Minus(vec2(0, imgui.CurrentStyle().FramePadding().Y-1))
		posLocal := imgui.CursorPos()

		imgui.Dummy(vec2(3, 0))
		imgui.SameLine()

		imgui.BeginGroup()

		editor.traverseChildren(jsonPath, sPath, u, d)

		imgui.EndGroup()

		pos1 := imgui.CursorScreenPos().Minus(vec2(0, imgui.CurrentStyle().ItemSpacing().Y))
		posLocal1 := imgui.CursorPos().Minus(vec2(0, imgui.CurrentStyle().ItemSpacing().Y))

		pos1.X = pos.X

		imgui.WindowDrawList().AddLine(pos, pos1, imgui.PackedColorFromVec4(vec4(1.0, 0.6, 1.0, 1.0)))

		imgui.TableSetColumnIndex(0)

		imgui.Dummy(vec2(1, 0))
		imgui.SameLine()

		ImIO.SetFontGlobalScale(0.625)
		imgui.PushFont(FontAw)

		imgui.SetCursorPos(vec2(imgui.CursorPosX(), (posLocal.Y+posLocal1.Y-imgui.FrameHeight())/2))

		removed = imgui.Button("\uF068" + jsonPath)

		ImIO.SetFontGlobalScale(1)
		imgui.PopFont()

		imgui.SameLine()
		imgui.Dummy(vec2(2, 0))

		imgui.EndTable()
	}

	return
}

func (editor *settingsEditor) traverseChildren(jsonPath, lPath string, u reflect.Value, d reflect.StructField) {
	typ := u.Elem()
	def := u.Type().Elem()

	if u.Type().AssignableTo(reflect.TypeOf(&settings.HSV{})) { // special case, if it's an array of colors we want to see color picker instead of Hue, Saturation and Value sliders
		editor.buildColor(jsonPath, u, d, false)
		return
	}

	count := typ.NumField()

	skipMap := make(map[string]uint8)
	consumed := make(map[string]uint8)

	for i, index := 0, 0; i < count; i++ {
		field := typ.Field(i)
		dF := def.Field(i)

		if (!field.CanInterface() && (!dF.Anonymous && dF.Tag.Get("vector") == "")) || dF.Tag.Get("skip") != "" {
			continue
		}

		if editor.shouldBeHidden(consumed, skipMap, typ, dF) {
			continue
		}

		label := editor.getLabel(def.Field(i))

		sPath2 := lPath + "." + label

		if editor.searchCache[sPath2] == 0 {
			continue
		}

		jsonPath1 := jsonPath + "." + dF.Name

		if tD, ok := dF.Tag.Lookup("json"); ok {
			sp := strings.Split(tD, ",")[0]

			if sp != "" {
				jsonPath1 = jsonPath + "." + sp
			}
		}

		if index > 0 {
			imgui.Dummy(vec2(0, 2))
		}

		switch field.Type().Kind() {
		case reflect.String:
			if _, ok := dF.Tag.Lookup("vector"); ok {
				lName, ok1 := dF.Tag.Lookup("left")
				rName, ok2 := dF.Tag.Lookup("right")
				if !ok1 || !ok2 {
					break
				}

				l := typ.FieldByName(lName)
				ld, _ := def.FieldByName(lName)

				r := typ.FieldByName(rName)
				rd, _ := def.FieldByName(rName)

				jsonPathL := jsonPath + "." + lName
				jsonPathR := jsonPath + "." + rName

				editor.buildVector(jsonPathL, jsonPathR, dF, l, ld, r, rd)
			} else {
				editor.buildString(jsonPath1, field, dF)
			}
		case reflect.Float64:
			editor.buildFloat(jsonPath1, field, dF)
		case reflect.Int64, reflect.Int, reflect.Int32:
			editor.buildInt(jsonPath1, field, dF)
		case reflect.Bool:
			editor.buildBool(jsonPath1, field, dF)
		case reflect.Slice:
			editor.buildArray(jsonPath1, sPath2, label, field, dF, index == 0, index == count-1)
		case reflect.Ptr:
			if field.Type().AssignableTo(reflect.TypeOf(&settings.HSV{})) {
				editor.buildColor(jsonPath1, field, dF, true)
			} else if !field.IsNil() {
				if dF.Anonymous {
					editor.traverseChildren(jsonPath, sPath2, field, dF)
				} else if field.CanInterface() {
					editor.buildSubSection(jsonPath1, sPath2, label, field, dF, index == 0, index == count-1)
				} else {
					index--
				}
			}
		default:
			index--
		}

		index++
	}
}

func (editor *settingsEditor) shouldBeHidden(consumed map[string]uint8, forceHide map[string]uint8, parent reflect.Value, currentSField reflect.StructField) bool {
	if _, ok := currentSField.Tag.Lookup("vector"); ok {
		lName, ok1 := currentSField.Tag.Lookup("left")
		rName, ok2 := currentSField.Tag.Lookup("right")
		if ok1 && ok2 {
			forceHide[lName] = 1
			forceHide[rName] = 1
		}
	}

	if forceHide[currentSField.Name] > 0 {
		return true
	}

	if s, ok := currentSField.Tag.Lookup("showif"); ok {
		s1 := strings.Split(s, "=")

		if s1[1] != "!" {
			fld := parent.FieldByName(s1[0])

			cF := fld.String()
			if fld.CanInt() {
				cF = strconv.Itoa(int(fld.Int()))
			} else if fld.Type().String() == "bool" {
				cF = "false"
				if fld.Bool() {
					cF = "true"
				}
			}

			found := false

			for _, toCheck := range strings.Split(s1[1], ",") {
				if toCheck[:1] == "!" {
					found = cF != toCheck[1:]

					if !found {
						break
					}
				} else if cF == toCheck {
					found = true
					break
				}
			}

			if !found {
				return true
			}

			consumed[s1[0]] = 1
		} else if consumed[s1[0]] == 1 {
			return true
		}
	}

	return false
}

func (editor *settingsEditor) getLabel(d reflect.StructField) string {
	if lb, ok := d.Tag.Lookup("label"); ok {
		return lb
	}

	dName := strings.Title(d.Name)

	parts := nameSplitter.FindAllString(dName, -1)
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToLower(parts[i])
	}

	return strings.Join(parts, " ")
}

func (editor *settingsEditor) buildBool(jsonPath string, f reflect.Value, d reflect.StructField) {
	editor.drawComponent(jsonPath, editor.getLabel(d), false, true, d, func() {
		base := f.Bool()

		if imgui.Checkbox(jsonPath, &base) {
			f.SetBool(base)
			editor.search()
		}
	})
}

func (editor *settingsEditor) buildVector(jsonPath1, jsonPath2 string, d reflect.StructField, l reflect.Value, ld reflect.StructField, r reflect.Value, rd reflect.StructField) {
	editor.drawComponent(jsonPath1+"\n"+jsonPath2, editor.getLabel(d), false, false, d, func() {
		contentAvail := imgui.ContentRegionAvail().X

		if imgui.BeginTableV("tv"+jsonPath1, 3, imgui.TableFlagsSizingStretchProp, vec2(contentAvail, 0), contentAvail) {
			imgui.TableSetupColumnV("tv1"+jsonPath1, imgui.TableColumnFlagsWidthStretch, 0, uint(0))
			imgui.TableSetupColumnV("tv2"+jsonPath1, imgui.TableColumnFlagsWidthFixed, 0, uint(1))
			imgui.TableSetupColumnV("tv3"+jsonPath1, imgui.TableColumnFlagsWidthStretch, 0, uint(2))

			imgui.TableNextColumn()

			imgui.SetNextItemWidth(-1)

			if l.CanInt() {
				editor.buildIntBox(jsonPath1, l, ld)
			} else {
				editor.buildFloatBox(jsonPath1, l, ld)
			}

			imgui.TableNextColumn()

			imgui.Text("x")

			imgui.TableNextColumn()

			imgui.SetNextItemWidth(-1)

			if r.CanInt() {
				editor.buildIntBox(jsonPath2, r, rd)
			} else {
				editor.buildFloatBox(jsonPath2, r, rd)
			}

			imgui.EndTable()
		}
	})
}

func (editor *settingsEditor) buildFloatBox(jsonPath string, f reflect.Value, d reflect.StructField) {
	min := float64(parseFloatOr(d.Tag.Get("min"), 0))
	max := float64(parseFloatOr(d.Tag.Get("max"), 1))
	scale := float64(parseFloatOr(d.Tag.Get("scale"), 1))

	base := f.Float()

	valSpeed := base * scale

	valText := strconv.FormatFloat(valSpeed, 'f', 2, 64)
	prevText := valText

	if imgui.InputTextV(jsonPath, &valText, imgui.InputTextFlagsCharsScientific, nil) {
		parsed, err := strconv.ParseFloat(valText, 64)
		if err != nil {
			valText = prevText
		} else {
			parsed = mutils.ClampF(parsed/scale, min, max)
			f.SetFloat(parsed)
		}
	}
}

func (editor *settingsEditor) buildIntBox(jsonPath string, f reflect.Value, d reflect.StructField) {
	min := parseIntOr(d.Tag.Get("min"), 0)
	max := parseIntOr(d.Tag.Get("max"), 100)

	base := int32(f.Int())

	if imgui.InputIntV(jsonPath, &base, 1, 1, 0) {
		base = mutils.Clamp(base, int32(min), int32(max))
		f.SetInt(int64(base))
	}
}

func (editor *settingsEditor) buildString(jsonPath string, f reflect.Value, d reflect.StructField) {
	editor.drawComponent(jsonPath, editor.getLabel(d), d.Tag.Get("long") != "", false, d, func() {
		imgui.SetNextItemWidth(-1)

		base := f.String()

		pDesc, okP := d.Tag.Lookup("path")
		fDesc, okF := d.Tag.Lookup("file")
		cSpec, okC := d.Tag.Lookup("combo")
		cFunc, okCS := d.Tag.Lookup("comboSrc")
		_, okPW := d.Tag.Lookup("password")

		if okP || okF {
			if imgui.BeginTableV("tbr"+jsonPath, 2, imgui.TableFlagsSizingStretchProp, vec2(-1, 0), -1) {
				imgui.TableSetupColumnV("tbr1"+jsonPath, imgui.TableColumnFlagsWidthStretch, 0, uint(0))
				imgui.TableSetupColumnV("tbr2"+jsonPath, imgui.TableColumnFlagsWidthFixed, 0, uint(1))

				imgui.TableNextColumn()

				imgui.SetNextItemWidth(-1)

				if imgui.InputText(jsonPath, &base) {
					f.SetString(base)
				}

				imgui.TableNextColumn()

				if imgui.Button("Browse" + jsonPath) {
					dir := getAbsPath(base)

					if strings.TrimSpace(base) != "" && okF {
						dir = filepath.Dir(dir)
					}

					if _, err := os.Lstat(dir); err != nil {
						dir = env.DataDir()
					}

					var p string
					var err error

					if okP {
						p, err = dialog.Directory().Title(pDesc).SetStartDir(dir).Browse()
					} else {
						spl := strings.Split(d.Tag.Get("filter"), "|")
						p, err = dialog.File().Title(fDesc).Filter(spl[0], strings.Split(spl[1], ",")...).SetStartDir(dir).Load()
					}

					if err == nil {
						oD := strings.TrimSuffix(strings.ReplaceAll(base, "\\", "/"), "/")
						nD := strings.TrimSuffix(strings.ReplaceAll(p, "\\", "/"), "/")

						if nD != oD {
							f.SetString(getRelativeOrABSPath(p))
						}
					}
				}

				imgui.EndTable()
			}
		} else if okC || okCS {
			var values []string
			var labels []string

			var options []string

			if okCS {
				options = reflect.ValueOf(settings.DefaultsFactory).MethodByName(cFunc).Call(nil)[0].Interface().([]string)
			} else {
				options = strings.Split(cSpec, ",")
			}

			lb := base

			for _, s := range options {
				splt := strings.Split(s, "|")

				optionLabel := splt[0]
				if len(splt) > 1 {
					optionLabel = splt[1]
				}

				values = append(values, splt[0])
				labels = append(labels, optionLabel)

				if base == splt[0] {
					lb = optionLabel
				}
			}

			if _, okSearch := d.Tag.Lookup("search"); okSearch {
				mWidth := imgui.CalcItemWidth() - imgui.CurrentStyle().FramePadding().X*2

				if imgui.BeginComboV(jsonPath, lb, imgui.ComboFlagsHeightLarge) {
					editor.blockSearch = true

					for _, s := range labels {
						mWidth = mutils.Max(mWidth, imgui.CalcTextSize(s, false, 0).X+20)
					}

					imgui.SetNextItemWidth(mWidth)

					cSearch := editor.comboSearch[jsonPath]

					focusScroll := searchBox("##search"+jsonPath, &cSearch)

					editor.comboSearch[jsonPath] = cSearch

					if !imgui.IsMouseClicked(0) && !imgui.IsMouseClicked(1) && !imgui.IsAnyItemActive() && !(imgui.IsWindowFocusedV(imgui.FocusedFlagsChildWindows) && !imgui.IsWindowFocused()) {
						imgui.SetKeyboardFocusHereV(-1)
					}

					imgui.PushStyleVarFloat(imgui.StyleVarFrameRounding, 0)
					imgui.PushStyleVarFloat(imgui.StyleVarFrameBorderSize, 0)
					imgui.PushStyleVarVec2(imgui.StyleVarFramePadding, vzero())
					imgui.PushStyleColor(imgui.StyleColorFrameBg, vec4(0, 0, 0, 0))

					searchResults := make([]string, 0, len(labels))
					searchValues := make([]string, 0, len(labels))

					search := strings.ToLower(cSearch)

					for i, s := range labels {
						if cSearch == "" || strings.Contains(strings.ToLower(s), search) {
							searchResults = append(searchResults, s)
							searchValues = append(searchValues, values[i])
						}
					}

					if len(searchResults) > 0 {
						sHeight := float32(mutils.Min(8, len(searchResults)))*imgui.FrameHeightWithSpacing() - imgui.CurrentStyle().ItemSpacing().Y/2

						if imgui.BeginListBoxV("##listbox"+jsonPath, vec2(mWidth, sHeight)) {
							focusScroll = focusScroll || imgui.IsWindowAppearing()

							for i, l := range searchResults {
								if selectableFocus(l+jsonPath, l == lb, focusScroll) {
									f.SetString(searchValues[i])
									editor.search()
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
			} else {
				if imgui.BeginCombo(jsonPath, lb) {
					justOpened := imgui.IsWindowAppearing()

					editor.blockSearch = true

					for i, l := range labels {
						if selectableFocus(l+jsonPath, l == lb, justOpened) {
							f.SetString(values[i])
							editor.search()
						}
					}

					imgui.EndCombo()
				}
			}
		} else if okPW {
			if imgui.BeginTableV("tpw"+jsonPath+"tb", 2, imgui.TableFlagsSizingStretchProp, vec2(-1, 0), -1) {
				imgui.TableSetupColumnV("tpw1"+jsonPath, imgui.TableColumnFlagsWidthStretch, 0, uint(0))
				imgui.TableSetupColumnV("tpw2"+jsonPath, imgui.TableColumnFlagsWidthFixed, 0, uint(1))

				show := editor.pwShowHide[jsonPath]

				iTFlags := imgui.InputTextFlagsNone
				if !show {
					iTFlags = imgui.InputTextFlagsPassword
				}

				imgui.TableNextColumn()

				imgui.SetNextItemWidth(-1)

				if imgui.InputTextV(jsonPath, &base, iTFlags, nil) {
					f.SetString(base)
				}

				imgui.TableNextColumn()

				tx := "Show"
				if show {
					tx = "Hide"
				}

				if imgui.ButtonV(tx+jsonPath, vec2(imgui.CalcTextSize("Show", false, 0).X+imgui.CurrentStyle().FramePadding().X*2, 0)) {
					editor.pwShowHide[jsonPath] = !editor.pwShowHide[jsonPath]
				}

				imgui.EndTable()
			}
		} else {
			if imgui.InputText(jsonPath, &base) {
				f.SetString(base)
			}
		}
	})
}

func (editor *settingsEditor) buildInt(jsonPath string, f reflect.Value, d reflect.StructField) {
	base := int32(f.Int())

	_, okS := d.Tag.Lookup("string")
	cSpec, okC := d.Tag.Lookup("combo")

	editor.drawComponent(jsonPath, editor.getLabel(d), !okS && !okC, false, d, func() {
		imgui.SetNextItemWidth(-1)

		format := firstOf(d.Tag.Get("format"), "%d")

		if okC {
			var values []int
			var labels []string

			lb := fmt.Sprintf(format, base)

			for _, s := range strings.Split(cSpec, ",") {
				splt := strings.Split(s, "|")
				c, _ := strconv.Atoi(splt[0])

				optionLabel := fmt.Sprintf(format, c)
				if len(splt) > 1 {
					optionLabel = splt[1]
				}

				values = append(values, c)
				labels = append(labels, optionLabel)

				if int(base) == c {
					lb = optionLabel
				}
			}

			if imgui.BeginCombo(jsonPath, lb) {
				justOpened := imgui.IsWindowAppearing()
				editor.blockSearch = true

				for i, l := range labels {
					if selectableFocus(l+jsonPath, l == lb, justOpened) {
						f.SetInt(int64(values[i]))
						editor.search()
					}
				}

				imgui.EndCombo()
			}
		} else if okS {
			editor.buildIntBox(jsonPath, f, d)
		} else {
			min := parseIntOr(d.Tag.Get("min"), 0)
			max := parseIntOr(d.Tag.Get("max"), 100)

			imgui.PushStyleVarVec2(imgui.StyleVarFramePadding, vec2(0, -3))

			if sliderIntSlide(jsonPath, &base, int32(min), int32(max), "##"+format, imgui.SliderFlagsNoInput) {
				f.SetInt(int64(base))
			}

			imgui.PopStyleVar()

			if imgui.IsItemHovered() || imgui.IsItemActive() {
				imgui.SetKeyboardFocusHereV(-1)
				editor.blockSearch = true

				imgui.BeginTooltip()
				imgui.SetTooltip(fmt.Sprintf(format, base))
				imgui.EndTooltip()
			}
		}
	})
}

func (editor *settingsEditor) buildFloat(jsonPath string, f reflect.Value, d reflect.StructField) {
	editor.drawComponent(jsonPath, editor.getLabel(d), d.Tag.Get("string") == "", false, d, func() {
		imgui.SetNextItemWidth(-1)

		if d.Tag.Get("string") != "" {
			editor.buildFloatBox(jsonPath, f, d)
		} else {
			min := parseFloatOr(d.Tag.Get("min"), 0)
			max := parseFloatOr(d.Tag.Get("max"), 1)
			scale := parseFloatOr(d.Tag.Get("scale"), 1)
			format := firstOf(d.Tag.Get("format"), "%.2f")

			base := float32(f.Float())
			valSpeed := base * scale

			imgui.PushStyleVarVec2(imgui.StyleVarFramePadding, vec2(0, -3))

			cSpacing := imgui.CurrentStyle().ItemSpacing()
			imgui.PushStyleVarVec2(imgui.StyleVarItemSpacing, vec2(cSpacing.X, cSpacing.Y-3))

			if sliderFloatSlide(jsonPath, &valSpeed, min*scale, max*scale, "##"+format, imgui.SliderFlagsNoInput) {
				f.SetFloat(float64(valSpeed / scale))
			}

			imgui.PopStyleVar()
			imgui.PopStyleVar()

			if imgui.IsItemHovered() || imgui.IsItemActive() {
				imgui.SetKeyboardFocusHereV(-1)
				editor.blockSearch = true

				imgui.BeginTooltip()
				imgui.SetTooltip(fmt.Sprintf(format, valSpeed))
				imgui.EndTooltip()
			}
		}
	})
}

func (editor *settingsEditor) buildColor(jsonPath string, f reflect.Value, d reflect.StructField, withLabel bool) {
	dComp := func() {
		imgui.SetNextItemWidth(imgui.ContentRegionAvail().X - 1)

		hsv := f.Interface().(*settings.HSV)

		r, g, b := color.HSVToRGB(float32(hsv.Hue), float32(hsv.Saturation), float32(hsv.Value))
		rgb := [3]float32{r, g, b}

		if imgui.ColorEdit3V(jsonPath, &rgb, imgui.ColorEditFlagsHSV|imgui.ColorEditFlagsNoLabel|imgui.ColorEditFlagsFloat) {
			h, s, v := color.RGBToHSV(rgb[0], rgb[1], rgb[2])
			hsv.Hue = float64(h)
			hsv.Saturation = float64(s)
			hsv.Value = float64(v)
		}

		editor.blockSearch = editor.blockSearch || imgui.IsWindowFocusedV(imgui.FocusedFlagsChildWindows) && !imgui.IsWindowFocused()
	}

	if withLabel {
		editor.drawComponent(jsonPath, editor.getLabel(d), false, false, d, dComp)
	} else {
		dComp()
	}
}

func (editor *settingsEditor) drawComponent(jsonPath, label string, long, checkbox bool, d reflect.StructField, draw func()) {
	width := imgui.FontSize() + imgui.CurrentStyle().FramePadding().X*2 - 1 // + imgui.CurrentStyle().ItemSpacing().X
	if !checkbox {
		width = 240 + imgui.CalcTextSize("x", false, 0).X + imgui.CurrentStyle().FramePadding().X*4
	}

	cCount := 1
	if !long {
		cCount = 2
	}

	contentAvail := imgui.ContentRegionAvail().X

	if imgui.BeginTableV("lbl"+jsonPath, cCount, imgui.TableFlagsSizingStretchProp|imgui.TableFlagsNoPadInnerX|imgui.TableFlagsNoPadOuterX|imgui.TableFlagsNoClip, vec2(contentAvail, 0), contentAvail) {
		if !long {
			imgui.TableSetupColumnV("lbl1"+jsonPath, imgui.TableColumnFlagsWidthFixed, contentAvail-width, uint(0))
			imgui.TableSetupColumnV("lbl2"+jsonPath, imgui.TableColumnFlagsWidthFixed, width, uint(1))
		} else {
			imgui.TableSetupColumnV("lbl1"+jsonPath, imgui.TableColumnFlagsWidthFixed, contentAvail, uint(0))
		}

		imgui.TableNextColumn()

		imgui.BeginGroup()
		imgui.AlignTextToFramePadding()
		imgui.Text(label)
		imgui.EndGroup()

		if imgui.IsItemHovered() {
			imgui.BeginTooltip()

			_, hPath := d.Tag.Lookup("hidePath")

			tTip := ""
			if !hPath {
				tTip = strings.ReplaceAll(jsonPath, "#", "")
			}

			if t, ok := d.Tag.Lookup("tooltip"); ok {
				if !hPath {
					tTip += "\n\n"
				}
				tTip += t
			}

			imgui.PushTextWrapPosV(400)

			imgui.Text(tTip)

			imgui.PopTextWrapPos()
			imgui.EndTooltip()
		}

		imgui.TableNextColumn()

		draw()

		imgui.EndTable()
	}
}

func parseIntOr(value string, alt int) int {
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}

	return alt
}

func parseFloatOr(value string, alt float32) float32 {
	if i, err := strconv.ParseFloat(value, 32); err == nil {
		return float32(i)
	}

	return alt
}

func firstOf(args ...string) string {
	for _, arg := range args {
		if arg != "" {
			return arg
		}
	}

	return ""
}
