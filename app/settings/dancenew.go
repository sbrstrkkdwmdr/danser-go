package settings

var CursorDance = initCursorDance()

func initCursorDance() *cursorDance {
	return &cursorDance{
		Movers: []*mover{
			DefaultsFactory.InitMover(),
		},
		Spinners: []*spinner{
			DefaultsFactory.InitSpinner(),
		},
		ComboTag:           false,
		Battle:             false,
		DoSpinnersTogether: true,
		TAGSliderDance:     false,
		MoverSettings: &moverSettings{
			Bezier: []*bezier{
				DefaultsFactory.InitBezier(),
			},
			Flower: []*flower{
				DefaultsFactory.InitFlower(),
			},
			HalfCircle: []*circular{
				DefaultsFactory.InitCircular(),
			},
			Spline: []*spline{
				DefaultsFactory.InitSpline(),
			},
			Momentum: []*momentum{
				DefaultsFactory.InitMomentum(),
			},
			ExGon: []*exgon{
				DefaultsFactory.InitExGon(),
			},
			Linear: []*linear{
				DefaultsFactory.InitLinear(),
			},
			Pippi: []*pippi{
				DefaultsFactory.InitPippi(),
			},
		},
		KeySettings: &keySettings{
			Type:             "normal",
			UseMouseInputs:   false,
			RepeatRandomKeys: false,
			SingleTapKey:     "k1",
			TapZXAlt:         false,
			StartWithK1:      false,
			PrimaryKey:       "k1",
			SingleTapThreshold: 140,
		},
	}
}

type mover struct {
	Mover             string `combo:"spline,bezier,circular,linear,axis,aggressive,flower,momentum,exgon,pippi"`
	SliderDance       bool
	RandomSliderDance bool
}

func (d *defaultsFactory) InitMover() *mover {
	return &mover{
		Mover:             "spline",
		SliderDance:       false,
		RandomSliderDance: false,
	}
}

type spinner struct {
	Mover         string  `combo:"heart,triangle,square,cube,circle"`
	centerOffset  string  `vector:"true" left:"CenterOffsetX" right:"CenterOffsetY"`
	CenterOffsetX float64 `min:"-1000" max:"1000"`
	CenterOffsetY float64 `min:"-1000" max:"1000"`
	Radius        float64 `max:"200" format:"%.0fo!px"`
}

func (d *defaultsFactory) InitSpinner() *spinner {
	return &spinner{
		Mover:  "circle",
		Radius: 100,
	}
}

type cursorDance struct {
	Movers             []*mover   `new:"InitMover"`
	Spinners           []*spinner `new:"InitSpinner"`
	ComboTag           bool       `liveedit:"false"`
	Battle             bool       `liveedit:"false"`
	DoSpinnersTogether bool       `liveedit:"false"`
	TAGSliderDance     bool       `label:"TAG slider dance" liveedit:"false"`
	MoverSettings      *moverSettings
	KeySettings        *keySettings
}

type moverSettings struct {
	Bezier     []*bezier   `new:"InitBezier"`
	Flower     []*flower   `new:"InitFlower"`
	HalfCircle []*circular `new:"InitCircular"`
	Spline     []*spline   `new:"InitSpline"`
	Momentum   []*momentum `new:"InitMomentum"`
	ExGon      []*exgon    `new:"InitExGon"`
	Linear     []*linear   `new:"InitLinear"`
	Pippi      []*pippi    `new:"InitPippi"`
}

type keySettings struct {
	Type               string  `label:"Key Input Type" combo:"normal|Default,alt|Alternating,single|Single Tap,tapx|Tap X,tapzx|Tap ZX,random|Random Key,descending|Descending,ascending|Ascending,bounce|Bouncing"`
	UseMouseInputs     bool    `label:"Use mouse inputs" showif:"Type=normal,alt" tooltip:"Use M1 and M2 instead of K1 and K2"`
	SingleTapThreshold float64 `min:"0" max:"300" showif:"Type=normal,tapx,tapzx" tooltip:"Time to wait before single tapping"`
	PrimaryKey         string  `combo:"k1,k2" showif:"Type=normal,alt" tooltip:"Whether to start tapping with K1 or K2"`
	SingleTapKey       string  `combo:"k1,k2,m1,m2" showif:"Type=single"`
	RepeatRandomKeys   bool    `label:"Allow random keys to repeat" showif:"Type=random"`
	TapZXAlt           bool    `label:"Use full alt when switching to key inputs" showif:"Type=tapzx"`
	StartWithK1        bool    `label:"Start with K1" showif:"Type=tapzx" tooltip:"Whether to start tapping with K1 or K2 when hitting streams"`
}
