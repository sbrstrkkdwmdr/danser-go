package input

import (
	"math/rand"
	"strings"

	"github.com/wieku/danser-go/app/beatmap/objects"
	"github.com/wieku/danser-go/app/dance/movers"
	"github.com/wieku/danser-go/app/graphics"
	"github.com/wieku/danser-go/app/settings"
	"github.com/wieku/danser-go/framework/math/mutils"
)

type keySettings struct {
	Type               string  `label:"Key Input Type" combo:"normal|Default,alt|Alternating,single|Single Tap,tapx|Tap X,tapzx|Tap ZX,random|Random Key,descending|Descending,ascending|Ascending,bounce|Bouncing,switch|Switching"`
	UseMouseInputs     bool    `label:"Use mouse inputs" showif:"Type=normal,alt" tooltip:"Use M1 and M2 instead of K1 and K2"`
	SingleTapThreshold float64 `min:"0" max:"300" showif:"Type=normal,tapx,tapzx" tooltip:"Time to wait before single tapping"`
	PrimaryKey         string  `combo:"k1,k2" showif:"Type=normal,alt" tooltip:"Whether to start tapping with K1 or K2"`
	SingleTapKey       string  `combo:"k1,k2,m1,m2" showif:"Type=single"`
	RepeatRandomKeys   bool    `label:"Allow random keys to repeat" showif:"Type=random"`
	TapZXAlt           bool    `label:"Use full alt when switching to key inputs" showif:"Type=tapzx"`
	StartWithK1        bool    `label:"Start with K1" showif:"Type=tapzx" tooltip:"Whether to start tapping with K1 or K2 when hitting streams"`
	MinSwitchInterval  int     `tooltip:"How many hits before danser can switch to another input type" showif:"Type=switch"`
	MaxSwitchInterval  int     `tooltip:"How many hits before danser has to switch to another input type" showif:"Type=switch"`
}

type NaturalInputProcessor struct {
	queue  []objects.IHitObject
	cursor *graphics.Cursor

	lastTime float64

	previousEnd     float64
	releaseLeftKAt  float64
	releaseRightKAt float64
	releaseLeftMAt  float64
	releaseRightMAt float64
	keyDirectionUp  bool
	mover           movers.MultiPointMover
	lastKey         int
	index           int

	switchIndex    int
	switchSettings *keySettings
}

func NewNaturalInputProcessor(objs []objects.IHitObject, cursor *graphics.Cursor, mover movers.MultiPointMover) *NaturalInputProcessor {
	processor := new(NaturalInputProcessor)
	processor.mover = mover
	processor.cursor = cursor
	processor.queue = make([]objects.IHitObject, len(objs))
	processor.releaseLeftKAt = -10000000
	processor.releaseRightKAt = -10000000
	processor.releaseLeftMAt = -10000000
	processor.releaseRightMAt = -10000000
	processor.lastKey = -1
	processor.index = -1
	processor.keyDirectionUp = true
	processor.switchIndex = 0
	processor.switchSettings = randomSettings();
	copy(processor.queue, objs)

	return processor
}

func (processor *NaturalInputProcessor) Update(time float64) {
	if len(processor.queue) > 0 {
		for i := 0; i < len(processor.queue); i++ {
			g := processor.queue[i]

			isDoubleClick := false
			if cC, ok := g.(*objects.Circle); ok && cC.DoubleClick {
				isDoubleClick = true
			}

			gStartTime := processor.mover.GetObjectsStartTime(g)
			gEndTime := processor.mover.GetObjectsEndTime(g)

			if gStartTime > time {
				break
			}

			if processor.lastTime < gStartTime && time >= gStartTime {
				startTime := gStartTime
				endTime := gEndTime

				releaseAt := endTime + 50.0

				if i+1 < len(processor.queue) {
					j := i + 1
					for ; j < len(processor.queue); j++ {
						// Prolong the click if slider tick is the next object
						if cC, ok := processor.queue[j].(*objects.Circle); ok && cC.SliderPoint && !cC.SliderPointStart {
							endTime = cC.GetEndTime()
							releaseAt = endTime + 50.0
						} else {
							break
						}
					}

					if j > i+1 {
						processor.queue = append(processor.queue[:i+1], processor.queue[j:]...)
					}

					if i+1 < len(processor.queue) {
						var obj objects.IHitObject

						// We want to depress earlier if current or next object is a double-click to have 2 keys free
						if nC, ok := processor.queue[i+1].(*objects.Circle); isDoubleClick || (ok && nC.DoubleClick) {
							obj = processor.queue[i+1]
						} else if i+2 < len(processor.queue) {
							obj = processor.queue[i+2]
						}

						if obj != nil {
							nTime := processor.mover.GetObjectsStartTime(obj)
							releaseAt = mutils.Clamp(nTime-1, endTime+1, releaseAt)
						}
					}
				}

				processor.previousEnd = endTime

				processor.queue = append(processor.queue[:i], processor.queue[i+1:]...)

				useSettings := (*keySettings)(settings.CursorDance.KeySettings)
				processor = processKeys(processor, isDoubleClick, startTime, releaseAt, useSettings)
				i--
			}
		}
	}

	processor.cursor.LeftKey = time < processor.releaseLeftKAt
	processor.cursor.RightKey = time < processor.releaseRightKAt
	processor.cursor.RightMouse = time < processor.releaseRightMAt
	processor.cursor.LeftMouse = time < processor.releaseLeftMAt

	processor.lastTime = time
}

func processKeys(processor *NaturalInputProcessor, isDoubleClick bool, startTime float64, releaseAt float64, useSettings *keySettings) *NaturalInputProcessor {
	switch useSettings.Type {
	default:
	case "normal": // default
		processor.index += 1
		shouldBeLeft := processor.index != 1 && startTime-processor.previousEnd < useSettings.SingleTapThreshold
		if isDoubleClick {
			processor.releaseLeftKAt = releaseAt
			processor.releaseRightKAt = releaseAt
		} else if shouldBeLeft {
			processor.releaseLeftKAt = releaseAt
			processor.index = 0
		} else {
			processor.releaseRightKAt = releaseAt
			processor.index = -1
		}
		processor = primaryKey(processor, useSettings)
		processor = mouseInputs(processor, useSettings)
	case "alt": // full alts
		processor.index += 1
		if processor.index%2 == 0 {
			processor.releaseRightKAt = releaseAt
		} else {
			processor.releaseLeftKAt = releaseAt
		}
		if processor.index >= 2 {
			processor.index = 0
		}
		processor = primaryKey(processor, useSettings)
		processor = mouseInputs(processor, useSettings)
	case "single":
		switch useSettings.SingleTapKey {
		case "k1":
			processor.releaseLeftKAt = releaseAt
		case "k2":
			processor.releaseRightKAt = releaseAt
		case "m1":
			processor.releaseLeftMAt = releaseAt
		case "m2":
			processor.releaseRightMAt = releaseAt
		}
		if isDoubleClick {
			if strings.Contains(useSettings.SingleTapKey, "k") {
				processor.releaseLeftKAt = releaseAt
				processor.releaseRightKAt = releaseAt
			} else {
				processor.releaseLeftMAt = releaseAt
				processor.releaseRightMAt = releaseAt
			}
		}
	case "tapx":
		processor.index += 1
		shouldBeLeft := processor.index != 1 && startTime-processor.previousEnd < useSettings.SingleTapThreshold
		if isDoubleClick {
			processor.releaseRightKAt = releaseAt
			processor.releaseLeftMAt = releaseAt
		} else if shouldBeLeft {
			processor.releaseRightKAt = releaseAt
			processor.index = 0
		} else {
			processor.releaseLeftMAt = releaseAt
			processor.index = -1
		}
	case "tapzx":
		isStream := startTime-processor.previousEnd < useSettings.SingleTapThreshold
		if isDoubleClick {
			processor.releaseRightKAt = releaseAt
			processor.releaseRightMAt = releaseAt
		} else if isStream {
			processor.index += 1
			if processor.index%2 == 0 {
				processor.releaseLeftKAt = releaseAt
			} else {
				processor.releaseRightKAt = releaseAt
			}
			if processor.index >= 2 {
				processor.index = 0
			}
		} else {
			processor.releaseLeftMAt = releaseAt
			if !useSettings.TapZXAlt {
				if useSettings.StartWithK1 {
					processor.index = 1
				} else {
					processor.index = 0
				}
			}
		}
	case "random": // picks a random key
		processor.index = randomKey(processor, useSettings)
		processor.lastKey = processor.index
		processor = specialKeys(processor, releaseAt, isDoubleClick)
	case "descending": // inputs "descend"
		processor.index += 1
		processor = specialKeys(processor, releaseAt, isDoubleClick)
		if processor.index >= 3 {
			processor.index = -1
		}
	case "ascending":
		if processor.index < 0 {
			processor.index = 4
		}
		processor.index -= 1
		processor = specialKeys(processor, releaseAt, isDoubleClick)
		if processor.index <= 0 {
			processor.index = 4
		}
	case "bounce":
		if processor.keyDirectionUp {
			processor.index += 1
			if processor.index >= 3 {
				processor.keyDirectionUp = false
			}
		} else {
			if processor.index < 0 {
				processor.index = 1
			}
			processor.index -= 1
			if processor.index <= 0 {
				processor.keyDirectionUp = true
			}
		}
		processor = specialKeys(processor, releaseAt, isDoubleClick)
	case "switch":
		processor.switchIndex += 1
		change := false

		if processor.switchIndex >= useSettings.MinSwitchInterval {
			change = rand.Float32() < 0.5
		}
		if processor.switchIndex >= useSettings.MaxSwitchInterval {
			change = true
		}
		if change {
			processor.lastKey = 1
			processor.index = 1
			processor.keyDirectionUp = true
			processor.switchIndex = 0
			processor.switchSettings = randomSettings();

		}
		processor = processKeys(processor, isDoubleClick, startTime, releaseAt, processor.switchSettings)

	}
	return processor
}

func randomKey(processor *NaturalInputProcessor, useSettings *keySettings) int {
	min := 0
	max := 4
	i := (rand.Intn(max - min))
	// prevent same key being tapped in a row
	if processor.lastKey == i && !useSettings.RepeatRandomKeys {
		return randomKey(processor, useSettings)
	} else {
		return i
	}
}

func specialKeys(processor *NaturalInputProcessor, releaseAt float64, isDoubleClick bool) *NaturalInputProcessor {
	if isDoubleClick {
		switch processor.index {
		case 0:
			processor.releaseLeftKAt = releaseAt
			processor.releaseRightKAt = releaseAt

		case 1:
			processor.releaseRightKAt = releaseAt
			processor.releaseLeftMAt = releaseAt

		case 2:
			processor.releaseLeftMAt = releaseAt
			processor.releaseRightMAt = releaseAt
		case 3:
			processor.releaseRightMAt = releaseAt
			processor.releaseLeftKAt = releaseAt
		}
	} else {
		switch processor.index {
		case 0:
			processor.releaseLeftKAt = releaseAt

		case 1:
			processor.releaseRightKAt = releaseAt
		case 2:
			processor.releaseLeftMAt = releaseAt
		case 3:
			processor.releaseRightMAt = releaseAt

		}
	}
	return processor
}
func primaryKey(processor *NaturalInputProcessor, useSettings *keySettings) *NaturalInputProcessor {
	if useSettings.PrimaryKey == "k1" {
		processor.releaseLeftKAt, processor.releaseRightKAt = processor.releaseRightKAt, processor.releaseLeftKAt
	}
	return processor
}

func mouseInputs(processor *NaturalInputProcessor, useSettings *keySettings) *NaturalInputProcessor {
	if useSettings.UseMouseInputs {
		processor.releaseRightMAt = processor.releaseLeftKAt
		processor.releaseLeftMAt = processor.releaseRightKAt
		processor.releaseLeftKAt = -10000000
		processor.releaseRightKAt = -10000000
	}
	return processor
}

func randomSettings() *keySettings {
	useSettings := &keySettings{}
	types := []string{"normal", "alt", "single", "tapx", "tapzx", "random", "descending", "ascending", "bounce"}
	useSettings.Type = types[(rand.Intn(len(types)))]
	useSettings.UseMouseInputs = rand.Float32() < 0.5
	useSettings.SingleTapThreshold = (rand.Float64() * (300 - 70)) + 70
	useKey := "k1"
	if rand.Float32() < 0.5 {
		useKey = "k2"
	}
	useSettings.PrimaryKey = useKey
	useSettings.SingleTapKey = useKey
	useSettings.RepeatRandomKeys = rand.Float32() < 0.5
	useSettings.TapZXAlt = rand.Float32() < 0.5
	useSettings.StartWithK1 = rand.Float32() < 0.5
	useSettings.MinSwitchInterval = settings.CursorDance.KeySettings.MinSwitchInterval
	useSettings.MaxSwitchInterval = settings.CursorDance.KeySettings.MaxSwitchInterval
	return useSettings;
}
