package input

import (
	"github.com/wieku/danser-go/app/beatmap/objects"
	"github.com/wieku/danser-go/app/dance/movers"
	"github.com/wieku/danser-go/app/graphics"
	"github.com/wieku/danser-go/framework/math/mutils"
)

type NaturalInputProcessor struct {
	queue  []objects.IHitObject
	cursor *graphics.Cursor

	lastTime float64

	wasLeftBefore  bool
	previousEnd    float64
	releaseLeftKAt  float64
	releaseRightKAt float64
	releaseLeftMAt float64
	releaseRightMAt float64
	mover          movers.MultiPointMover
	index int32
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
processor.index = 0

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

				processor.index+=1;
				if(processor.index > 3){
					processor.index = 0;
				}

				if isDoubleClick {
					switch(processor.index){
					case 0:default:
						processor.releaseLeftKAt = releaseAt
						processor.releaseRightKAt = releaseAt
					case 1:
						processor.releaseRightKAt = releaseAt;
						processor.releaseLeftMAt = releaseAt;
					case 2:
						processor.releaseLeftMAt = releaseAt;
						processor.releaseRightMAt = releaseAt;
					case 3:
						processor.releaseRightMAt = releaseAt;
						processor.releaseLeftKAt = releaseAt
					}
				} else  {
					switch(processor.index){
					case 0:default:
						processor.releaseLeftKAt = releaseAt;
					case 1:
						processor.releaseRightKAt = releaseAt;
					case 2:
						processor.releaseLeftMAt = releaseAt;
					case 3:
						processor.releaseRightMAt = releaseAt;
					}
				}

				processor.previousEnd = endTime

				processor.queue = append(processor.queue[:i], processor.queue[i+1:]...)

				i--
			}
		}
	}

	processor.cursor.LeftKey = time < processor.releaseLeftKAt
	processor.cursor.RightKey = time < processor.releaseRightKAt
	processor.cursor.RightMouse = time < processor.releaseLeftMAt
	processor.cursor.LeftMouse = time < processor.releaseRightMAt

	processor.lastTime = time
}
