package output

import "unicode"

// wide holds the characters Unicode 17 marks East Asian Wide or Fullwidth,
// generated from golang.org/x/text/width so the CLI does not depend on it.
// Folding in unassigned code points between ranges keeps the table short.
var wide = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115f, Stride: 1},
		{Lo: 0x231a, Hi: 0x231b, Stride: 1},
		{Lo: 0x2329, Hi: 0x232a, Stride: 1},
		{Lo: 0x23e9, Hi: 0x23ec, Stride: 1},
		{Lo: 0x23f0, Hi: 0x23f0, Stride: 1},
		{Lo: 0x23f3, Hi: 0x23f3, Stride: 1},
		{Lo: 0x25fd, Hi: 0x25fe, Stride: 1},
		{Lo: 0x2614, Hi: 0x2615, Stride: 1},
		{Lo: 0x2630, Hi: 0x2637, Stride: 1},
		{Lo: 0x2648, Hi: 0x2653, Stride: 1},
		{Lo: 0x267f, Hi: 0x267f, Stride: 1},
		{Lo: 0x268a, Hi: 0x268f, Stride: 1},
		{Lo: 0x2693, Hi: 0x2693, Stride: 1},
		{Lo: 0x26a1, Hi: 0x26a1, Stride: 1},
		{Lo: 0x26aa, Hi: 0x26ab, Stride: 1},
		{Lo: 0x26bd, Hi: 0x26be, Stride: 1},
		{Lo: 0x26c4, Hi: 0x26c5, Stride: 1},
		{Lo: 0x26ce, Hi: 0x26ce, Stride: 1},
		{Lo: 0x26d4, Hi: 0x26d4, Stride: 1},
		{Lo: 0x26ea, Hi: 0x26ea, Stride: 1},
		{Lo: 0x26f2, Hi: 0x26f3, Stride: 1},
		{Lo: 0x26f5, Hi: 0x26f5, Stride: 1},
		{Lo: 0x26fa, Hi: 0x26fa, Stride: 1},
		{Lo: 0x26fd, Hi: 0x26fd, Stride: 1},
		{Lo: 0x2705, Hi: 0x2705, Stride: 1},
		{Lo: 0x270a, Hi: 0x270b, Stride: 1},
		{Lo: 0x2728, Hi: 0x2728, Stride: 1},
		{Lo: 0x274c, Hi: 0x274c, Stride: 1},
		{Lo: 0x274e, Hi: 0x274e, Stride: 1},
		{Lo: 0x2753, Hi: 0x2755, Stride: 1},
		{Lo: 0x2757, Hi: 0x2757, Stride: 1},
		{Lo: 0x2795, Hi: 0x2797, Stride: 1},
		{Lo: 0x27b0, Hi: 0x27b0, Stride: 1},
		{Lo: 0x27bf, Hi: 0x27bf, Stride: 1},
		{Lo: 0x2b1b, Hi: 0x2b1c, Stride: 1},
		{Lo: 0x2b50, Hi: 0x2b50, Stride: 1},
		{Lo: 0x2b55, Hi: 0x2b55, Stride: 1},
		{Lo: 0x2e80, Hi: 0x303e, Stride: 1},
		{Lo: 0x3041, Hi: 0x3247, Stride: 1},
		{Lo: 0x3250, Hi: 0xa4c6, Stride: 1},
		{Lo: 0xa960, Hi: 0xa97c, Stride: 1},
		{Lo: 0xac00, Hi: 0xd7a3, Stride: 1},
		{Lo: 0xf900, Hi: 0xfaff, Stride: 1},
		{Lo: 0xfe10, Hi: 0xfe19, Stride: 1},
		{Lo: 0xfe30, Hi: 0xfe6b, Stride: 1},
		{Lo: 0xff01, Hi: 0xff60, Stride: 1},
		{Lo: 0xffe0, Hi: 0xffe6, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x16fe0, Hi: 0x1b2fb, Stride: 1},
		{Lo: 0x1d300, Hi: 0x1d376, Stride: 1},
		{Lo: 0x1f004, Hi: 0x1f004, Stride: 1},
		{Lo: 0x1f0cf, Hi: 0x1f0cf, Stride: 1},
		{Lo: 0x1f18e, Hi: 0x1f18e, Stride: 1},
		{Lo: 0x1f191, Hi: 0x1f19a, Stride: 1},
		{Lo: 0x1f200, Hi: 0x1f320, Stride: 1},
		{Lo: 0x1f32d, Hi: 0x1f335, Stride: 1},
		{Lo: 0x1f337, Hi: 0x1f37c, Stride: 1},
		{Lo: 0x1f37e, Hi: 0x1f393, Stride: 1},
		{Lo: 0x1f3a0, Hi: 0x1f3ca, Stride: 1},
		{Lo: 0x1f3cf, Hi: 0x1f3d3, Stride: 1},
		{Lo: 0x1f3e0, Hi: 0x1f3f0, Stride: 1},
		{Lo: 0x1f3f4, Hi: 0x1f3f4, Stride: 1},
		{Lo: 0x1f3f8, Hi: 0x1f43e, Stride: 1},
		{Lo: 0x1f440, Hi: 0x1f440, Stride: 1},
		{Lo: 0x1f442, Hi: 0x1f4fc, Stride: 1},
		{Lo: 0x1f4ff, Hi: 0x1f53d, Stride: 1},
		{Lo: 0x1f54b, Hi: 0x1f54e, Stride: 1},
		{Lo: 0x1f550, Hi: 0x1f567, Stride: 1},
		{Lo: 0x1f57a, Hi: 0x1f57a, Stride: 1},
		{Lo: 0x1f595, Hi: 0x1f596, Stride: 1},
		{Lo: 0x1f5a4, Hi: 0x1f5a4, Stride: 1},
		{Lo: 0x1f5fb, Hi: 0x1f64f, Stride: 1},
		{Lo: 0x1f680, Hi: 0x1f6c5, Stride: 1},
		{Lo: 0x1f6cc, Hi: 0x1f6cc, Stride: 1},
		{Lo: 0x1f6d0, Hi: 0x1f6d2, Stride: 1},
		{Lo: 0x1f6d5, Hi: 0x1f6df, Stride: 1},
		{Lo: 0x1f6eb, Hi: 0x1f6ec, Stride: 1},
		{Lo: 0x1f6f4, Hi: 0x1f6fc, Stride: 1},
		{Lo: 0x1f7e0, Hi: 0x1f7f0, Stride: 1},
		{Lo: 0x1f90c, Hi: 0x1f93a, Stride: 1},
		{Lo: 0x1f93c, Hi: 0x1f945, Stride: 1},
		{Lo: 0x1f947, Hi: 0x1f9ff, Stride: 1},
		{Lo: 0x1fa70, Hi: 0x1faf8, Stride: 1},
		{Lo: 0x20000, Hi: 0x3ffff, Stride: 1},
	},
}

// zeroWidth holds characters a terminal draws over the previous cell:
// combining marks, plus the Hangul vowels and final consonants that join the
// syllable before them.
var zeroWidth = []*unicode.RangeTable{unicode.Mn, unicode.Me, {
	R16: []unicode.Range16{{Lo: 0x1160, Hi: 0x11ff, Stride: 1}},
}}

func runeWidth(r rune) int {
	switch {
	case unicode.IsOneOf(zeroWidth, r):
		return 0
	case unicode.Is(wide, r):
		return 2
	}
	return 1
}

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}
