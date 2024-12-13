package settings

var CursorDance = initCursorDance()

func initCursorDance() *cursorDance {
	return &cursorDance{
		KeyInType:       "normal",
		KeyMouse:        "k",
		KeyRandomRepeat: false,
		SingleTapKey:    "k1",
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
	KeyInType		   string     `label:"Key Input Type" combo:"normal|Default,alt|Alternating,single|Single Tap,tapx|Tap X,tapzx|Tap ZX,random|Random Key,descending|Descending,ascending|Ascending,bounce|Bouncing"`
	KeyMouse 		   string     `label:"Use key or mouse inputs" combo:"k|Keyboard,m|Mouse" showif:"KeyInType=normal,alt"`
	SingleTapKey       string      `combo:"k1,k2,m1,m2" showif:"KeyInType=single"`
	KeyRandomRepeat    bool       `label:"Allow random keys to repeat" showif:"KeyInType=random"`
	Movers             []*mover   `new:"InitMover"`
	Spinners           []*spinner `new:"InitSpinner"`
	ComboTag           bool       `liveedit:"false"`
	Battle             bool       `liveedit:"false"`
	DoSpinnersTogether bool       `liveedit:"false"`
	TAGSliderDance     bool       `label:"TAG slider dance" liveedit:"false"`
	MoverSettings      *moverSettings
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
