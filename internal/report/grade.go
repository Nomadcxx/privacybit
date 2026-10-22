package report

import "github.com/Nomadcxx/privacybit/internal/catalog"

type GradeRender struct {
	Color string
	ANSI  int
	Glyph string
	ASCII string
	Word  string
}

// GradeChannels returns the shared colour, shape, and text representation.
func GradeChannels(grade catalog.Grade) GradeRender {
	switch grade {
	case catalog.GradeGreen:
		return GradeRender{Color: "green", ANSI: 32, Glyph: "●", ASCII: "o", Word: "Good"}
	case catalog.GradeAmber:
		return GradeRender{Color: "yellow", ANSI: 33, Glyph: "▲", ASCII: "^", Word: "Review"}
	case catalog.GradeRed:
		return GradeRender{Color: "red", ANSI: 31, Glyph: "■", ASCII: "x", Word: "Risky"}
	default:
		return GradeRender{Color: "muted", Glyph: "?", ASCII: "?", Word: "Unrated"}
	}
}
