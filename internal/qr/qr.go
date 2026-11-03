// Package qr — QR-код (ISO/IEC 18004) для ссылки на пульт с телефона: байтовый режим, версии 1–40,
// автоматический выбор версии и маски. Алгоритм — как в известном генераторе Nayuki (qrcodegen).
package qr

import (
	"errors"
	"fmt"
	"strings"
)

// Level — уровень коррекции ошибок.
type Level int

const (
	L Level = iota // ~7 %
	M              // ~15 %
	Q              // ~25 %
	H              // ~30 %
)

// formatBits — код уровня в служебной информации (не совпадает с порядком L, M, Q, H).
var formatBits = [4]int{1, 0, 3, 2}

var eccPerBlock = [4][41]int{
	{-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

var numBlocks = [4][41]int{
	{-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

// Code — готовый QR-код: Size×Size модулей, true — тёмный.
type Code struct {
	Version int
	Size    int
	Level   Level
	Mask    int
	mod     [][]bool
	fn      [][]bool // служебные модули (не данные)
}

// Dark — тёмный ли модуль (x — столбец, y — строка). За краями — светлые поля.
func (c *Code) Dark(x, y int) bool {
	return x >= 0 && y >= 0 && x < c.Size && y < c.Size && c.mod[y][x]
}

// Encode кодирует текст (UTF-8, байтовый режим) наименьшей подходящей версией.
// Уровень коррекции повышается, если это не увеличивает размер.
func Encode(text string, lvl Level) (*Code, error) {
	data := []byte(text)
	ver := 0
	for v := 1; v <= 40; v++ {
		if len(data) <= capacity(v, lvl) {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, errors.New("qr: слишком длинный текст")
	}
	for l := lvl + 1; l <= H; l++ { // бесплатный запас прочности
		if len(data) <= capacity(ver, l) {
			lvl = l
		}
	}
	return encodeVersion(data, ver, lvl), nil
}

func countBits(ver int) int {
	if ver <= 9 {
		return 8
	}
	return 16
}

// capacity — сколько байт помещается в версию ver.
func capacity(ver int, lvl Level) int {
	bits := dataCodewords(ver, lvl)*8 - 4 - countBits(ver)
	return bits / 8
}

func rawModules(ver int) int {
	r := (16*ver+128)*ver + 64
	if ver >= 2 {
		n := ver/7 + 2
		r -= (25*n-10)*n - 55
		if ver >= 7 {
			r -= 36
		}
	}
	return r
}

func dataCodewords(ver int, lvl Level) int {
	return rawModules(ver)/8 - eccPerBlock[lvl][ver]*numBlocks[lvl][ver]
}

type bitBuf []bool

func (b *bitBuf) add(v, n int) {
	for i := n - 1; i >= 0; i-- {
		*b = append(*b, (v>>i)&1 != 0)
	}
}

func encodeVersion(data []byte, ver int, lvl Level) *Code {
	capBits := dataCodewords(ver, lvl) * 8
	var bb bitBuf
	bb.add(0x4, 4) // байтовый режим
	bb.add(len(data), countBits(ver))
	for _, b := range data {
		bb.add(int(b), 8)
	}
	bb.add(0, min(4, capBits-len(bb)))
	bb.add(0, (8-len(bb)%8)%8)
	for pad := 0xEC; len(bb) < capBits; pad ^= 0xEC ^ 0x11 {
		bb.add(pad, 8)
	}
	words := make([]byte, len(bb)/8)
	for i, bit := range bb {
		if bit {
			words[i>>3] |= 1 << (7 - uint(i&7))
		}
	}
	c := &Code{Version: ver, Size: ver*4 + 17, Level: lvl}
	c.mod = make([][]bool, c.Size)
	c.fn = make([][]bool, c.Size)
	for i := range c.mod {
		c.mod[i] = make([]bool, c.Size)
		c.fn[i] = make([]bool, c.Size)
	}
	c.drawFunctionPatterns()
	c.drawCodewords(c.addECC(words))
	best, bestScore := 0, -1
	for m := 0; m < 8; m++ {
		c.applyMask(m)
		c.drawFormat(m)
		if s := c.penalty(); bestScore < 0 || s < bestScore {
			best, bestScore = m, s
		}
		c.applyMask(m) // XOR — снять маску
	}
	c.Mask = best
	c.applyMask(best)
	c.drawFormat(best)
	return c
}

func (c *Code) setFn(x, y int, dark bool) {
	c.mod[y][x] = dark
	c.fn[y][x] = true
}

func (c *Code) drawFunctionPatterns() {
	n := c.Size
	for i := 0; i < n; i++ {
		c.setFn(6, i, i%2 == 0)
		c.setFn(i, 6, i%2 == 0)
	}
	c.finder(3, 3)
	c.finder(n-4, 3)
	c.finder(3, n-4)
	pos := c.alignPositions()
	k := len(pos)
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			if i == 0 && j == 0 || i == 0 && j == k-1 || i == k-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.setFn(pos[i]+dx, pos[j]+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	c.drawFormat(0) // место под служебную информацию (перепишется)
	c.drawVersion()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (c *Code) finder(x, y int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			d := max(abs(dx), abs(dy))
			xx, yy := x+dx, y+dy
			if xx >= 0 && xx < c.Size && yy >= 0 && yy < c.Size {
				c.setFn(xx, yy, d != 2 && d != 4)
			}
		}
	}
}

func (c *Code) alignPositions() []int {
	if c.Version == 1 {
		return nil
	}
	n := c.Version/7 + 2
	step := (c.Version*8 + n*3 + 5) / (n*4 - 4) * 2
	out := make([]int, n)
	out[0] = 6
	for i, p := n-1, c.Size-7; i >= 1; i, p = i-1, p-step {
		out[i] = p
	}
	return out
}

func (c *Code) drawFormat(mask int) {
	data := formatBits[c.Level]<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>uint(i))&1 != 0 }
	for i := 0; i <= 5; i++ {
		c.setFn(8, i, bit(i))
	}
	c.setFn(8, 7, bit(6))
	c.setFn(8, 8, bit(7))
	c.setFn(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.setFn(14-i, 8, bit(i))
	}
	n := c.Size
	for i := 0; i < 8; i++ {
		c.setFn(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.setFn(8, n-15+i, bit(i))
	}
	c.setFn(8, n-8, true)
}

func (c *Code) drawVersion() {
	if c.Version < 7 {
		return
	}
	rem := c.Version
	for i := 0; i < 12; i++ {
		rem = rem<<1 ^ (rem>>11)*0x1F25
	}
	bits := c.Version<<12 | rem
	for i := 0; i < 18; i++ {
		dark := (bits>>uint(i))&1 != 0
		a, b := c.Size-11+i%3, i/3
		c.setFn(a, b, dark)
		c.setFn(b, a, dark)
	}
}

// addECC — блоки с кодами Рида — Соломона, перемешанные по стандарту.
func (c *Code) addECC(data []byte) []byte {
	nb := numBlocks[c.Level][c.Version]
	eccLen := eccPerBlock[c.Level][c.Version]
	raw := rawModules(c.Version) / 8
	short := nb - raw%nb
	shortLen := raw / nb
	div := rsDivisor(eccLen)
	blocks := make([][]byte, nb)
	k := 0
	for i := 0; i < nb; i++ {
		n := shortLen - eccLen
		if i >= short {
			n++
		}
		dat := data[k : k+n]
		k += n
		block := make([]byte, shortLen+1)
		copy(block, dat)
		copy(block[len(block)-eccLen:], rsRemainder(dat, div))
		blocks[i] = block
	}
	out := make([]byte, 0, raw)
	for i := 0; i < len(blocks[0]); i++ {
		for j := range blocks {
			if i != shortLen-eccLen || j >= short {
				out = append(out, blocks[j][i])
			}
		}
	}
	return out
}

func rsMul(x, y int) int {
	z := 0
	for i := 7; i >= 0; i-- {
		z = z<<1 ^ (z>>7)*0x11D
		z ^= (y >> uint(i) & 1) * x
	}
	return z
}

func rsDivisor(deg int) []byte {
	res := make([]byte, deg)
	res[deg-1] = 1
	root := 1
	for i := 0; i < deg; i++ {
		for j := range res {
			res[j] = byte(rsMul(int(res[j]), root))
			if j+1 < len(res) {
				res[j] ^= res[j+1]
			}
		}
		root = rsMul(root, 2)
	}
	return res
}

func rsRemainder(data, div []byte) []byte {
	res := make([]byte, len(div))
	for _, b := range data {
		f := int(b ^ res[0])
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i := range res {
			res[i] ^= byte(rsMul(int(div[i]), f))
		}
	}
	return res
}

func (c *Code) drawCodewords(data []byte) {
	i := 0
	n := c.Size
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if !c.fn[y][x] && i < len(data)*8 {
					c.mod[y][x] = (data[i>>3]>>(7-uint(i&7)))&1 != 0
					i++
				}
			}
		}
	}
}

func (c *Code) applyMask(m int) {
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			var inv bool
			switch m {
			case 0:
				inv = (x+y)%2 == 0
			case 1:
				inv = y%2 == 0
			case 2:
				inv = x%3 == 0
			case 3:
				inv = (x+y)%3 == 0
			case 4:
				inv = (x/3+y/2)%2 == 0
			case 5:
				inv = x*y%2+x*y%3 == 0
			case 6:
				inv = (x*y%2+x*y%3)%2 == 0
			case 7:
				inv = ((x+y)%2+x*y%3)%2 == 0
			}
			if inv && !c.fn[y][x] {
				c.mod[y][x] = !c.mod[y][x]
			}
		}
	}
}

// penalty — штраф маски по правилам стандарта (меньше — легче сканировать).
func (c *Code) penalty() int {
	n := c.Size
	score := 0
	line := func(get func(i int) bool) {
		run := 1
		for i := 1; i <= n; i++ {
			if i < n && get(i) == get(i-1) {
				run++
				continue
			}
			if run >= 5 {
				score += 3 + run - 5
			}
			run = 1
		}
		// 1:1:3:1:1 с четырьмя светлыми с одной стороны (за краем — светлое поле)
		pat := []bool{true, false, true, true, true, false, true}
		at := func(i int) bool { return i >= 0 && i < n && get(i) }
		for i := -4; i < n; i++ {
			ok := true
			for k, p := range pat {
				if at(i+k) != p {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			before, after := true, true
			for k := 1; k <= 4; k++ {
				before = before && !at(i-k)
				after = after && !at(i+6+k)
			}
			if before || after {
				score += 40
			}
		}
	}
	for y := 0; y < n; y++ {
		line(func(i int) bool { return c.mod[y][i] })
	}
	for x := 0; x < n; x++ {
		line(func(i int) bool { return c.mod[i][x] })
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if c.mod[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				v := c.mod[y][x]
				if c.mod[y][x+1] == v && c.mod[y+1][x] == v && c.mod[y+1][x+1] == v {
					score += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return score + k*10
}

// SVG — картинка с полем 4 модуля: тёмные модули одним контуром (чётко при любом масштабе).
func (c *Code) SVG(dark, light string) string {
	if dark == "" {
		dark = "#000"
	}
	if light == "" {
		light = "#fff"
	}
	const border = 4
	n := c.Size + 2*border
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, n, n)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/><path fill="%s" d="`, n, n, light, dark)
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if !c.mod[y][x] {
				continue
			}
			w := 1 // соседние тёмные в строке — одним прямоугольником
			for x+w < c.Size && c.mod[y][x+w] {
				w++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+border, y+border, w, w)
			x += w - 1
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}
