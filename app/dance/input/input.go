package input

import (
	"math/rand"

	"github.com/wieku/danser-go/app/beatmap/objects"
	"github.com/wieku/danser-go/app/dance/movers"
	"github.com/wieku/danser-go/app/graphics"
	"github.com/wieku/danser-go/app/settings"
	"github.com/wieku/danser-go/framework/math/mutils"
)

type NaturalInputProcessor struct {
	queue  []objects.IHitObject
	cursor *graphics.Cursor

	lastTime float64

	previousEnd    float64
	releaseLeftKAt  float64
	releaseRightKAt float64
	releaseLeftMAt float64
	releaseRightMAt float64
	keyDirectionUp bool
	mover          movers.MultiPointMover
	lastKey int
	index int
}

const singleTapThreshold = 140

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

				switch (settings.CursorDance.KeyInType) {
					default:case "normal": // default
					processor.index+=1;
					shouldBeLeft := processor.index != 1 && startTime-processor.previousEnd < singleTapThreshold

				if isDoubleClick {
					processor.releaseLeftKAt = releaseAt
					processor.releaseRightKAt = releaseAt
				} else if shouldBeLeft {
					processor.releaseLeftKAt = releaseAt
					processor.index = 0;
				} else {
					processor.releaseRightKAt = releaseAt
					processor.index = -1;
				}
				processor = mouseInputs(processor);
				case "alt": // full alts 
					processor.index+=1;
					if(processor.index % 2 == 0){
						processor.releaseLeftKAt = releaseAt;
					} else {
						processor.releaseRightKAt = releaseAt;
					}
					if(processor.index >=2){
						processor.index = 0;
					}
				processor = mouseInputs(processor);
				case "random": // picks a random key
					processor.index = randomKey(processor);
					processor.lastKey = processor.index;
					processor = processKeys(processor, releaseAt, isDoubleClick);
				case "descending": // inputs "descend"
					processor.index+=1;
					processor = processKeys(processor, releaseAt, isDoubleClick);
					if(processor.index >= 3){
						processor.index = -1;
					}
				case "ascending": 
					processor.index-=1;
					processor = processKeys(processor, releaseAt, isDoubleClick);
					if(processor.index <= 0){
						processor.index = 4;
					}
				case "bounce":
					if(processor.keyDirectionUp){
					processor.index+=1;
					if(processor.index >= 3){
						processor.keyDirectionUp = false;
					}
					} else {
						processor.index-=1;
						if(processor.index <= 0){
							processor.keyDirectionUp = true;
						}
					}
					processor = processKeys(processor, releaseAt, isDoubleClick);
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

func randomKey(processor *NaturalInputProcessor) int {
	min := 0;
	max := 4;
	i := (rand.Intn(max - min));
	// prevent same key being tapped in a row
	if(processor.lastKey == i && !settings.CursorDance.KeyRandomRepeat){
		return randomKey(processor);
	} else {
		return i;
	}
}

func processKeys(processor *NaturalInputProcessor, releaseAt float64, isDoubleClick bool) *NaturalInputProcessor {
	if isDoubleClick {
		switch(processor.index){
		case 0:
			processor.releaseLeftKAt = releaseAt;
			processor.releaseRightKAt = releaseAt;
			
		case 1:
			processor.releaseRightKAt = releaseAt;
			processor.releaseRightMAt = releaseAt;
			
		case 2:
			processor.releaseLeftMAt = releaseAt;
			processor.releaseRightMAt = releaseAt;
			
		case 3:
			processor.releaseLeftMAt = releaseAt;
			processor.releaseLeftKAt = releaseAt;
		}
	} else  {
		switch(processor.index){
		case 0:
			processor.releaseLeftKAt = releaseAt;
			
		case 1:
			processor.releaseRightKAt = releaseAt;
			
		case 2:
			processor.releaseRightMAt = releaseAt;
		case 3:
			processor.releaseLeftMAt = releaseAt;
			
		}
	}
	return processor;
}

func mouseInputs(processor *NaturalInputProcessor) *NaturalInputProcessor {
	if(settings.CursorDance.KeyMouse == "m"){
		processor.releaseLeftMAt = processor.releaseLeftKAt;
		processor.releaseRightMAt = processor.releaseRightKAt;
		processor.releaseLeftKAt = -10000000;
		processor.releaseRightKAt= -10000000;
	}
	return processor;
}