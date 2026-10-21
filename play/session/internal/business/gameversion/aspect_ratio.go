package gameversion

// AspectRatio is the fixed shape a game version's frontend is shown in. A
// game picks one of the ratios Playhoot offers; it cannot declare its own.
type AspectRatio string

const (
	AspectRatio16x9 AspectRatio = "16:9"
	AspectRatio9x16 AspectRatio = "9:16"
	AspectRatio4x3  AspectRatio = "4:3"
	AspectRatio3x4  AspectRatio = "3:4"
	AspectRatio1x1  AspectRatio = "1:1"
)

// Resolution is the logical size a frontend draws in and receives input in,
// whatever the device showing it.
type Resolution struct {
	Width  int
	Height int
}

// resolutions is the catalog of offered ratios, 1080 logical units on the
// short side.
var resolutions = map[AspectRatio]Resolution{
	AspectRatio16x9: {Width: 1920, Height: 1080},
	AspectRatio9x16: {Width: 1080, Height: 1920},
	AspectRatio4x3:  {Width: 1440, Height: 1080},
	AspectRatio3x4:  {Width: 1080, Height: 1440},
	AspectRatio1x1:  {Width: 1080, Height: 1080},
}

// Resolution returns r's logical resolution; ok is false when Playhoot does
// not offer r.
func (r AspectRatio) Resolution() (resolution Resolution, ok bool) {
	resolution, ok = resolutions[r]
	return resolution, ok
}
