package report

import (
	"testing"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

func TestGradeChannelsKeepWordShapeAndColorTogether(t *testing.T) {
	for _, tc := range []struct {
		grade catalog.Grade
		word  string
		glyph string
		ascii string
		ansi  int
	}{
		{catalog.GradeGreen, "Good", "●", "o", 32},
		{catalog.GradeAmber, "Review", "▲", "^", 33},
		{catalog.GradeRed, "Risky", "■", "x", 31},
		{catalog.GradeUnknown, "Unrated", "?", "?", 0},
	} {
		got := GradeChannels(tc.grade)
		if got.Word != tc.word || got.Glyph != tc.glyph || got.ASCII != tc.ascii || got.ANSI != tc.ansi {
			t.Errorf("GradeChannels(%s) = %+v", tc.grade, got)
		}
	}
}
