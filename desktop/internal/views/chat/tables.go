package chat

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	tableBorder        = 1
	tableToolbarHeight = 40
	tableToolbarPadX   = 4
	tableToolbarGap    = 4
	tableCellPadX      = 16
	tableCellPadY      = 8
	tableHeaderSize    = 12
	tableWordCap       = 24 * bodySize
	fullscreenPadding  = 16
	fullscreenFade     = 160 * time.Millisecond
	closeGlyph         = "x"
	tableFileBase      = "table"
	tableFileCSV       = "csv"
	tableFileMarkdown  = "md"
)

var inlineCodeRadius = theme.Concentric(theme.RadiusInset, 4)

type tableData struct {
	align []parser.Align
	rows  [][][]parser.Inline
}

type tableFormat struct {
	label  func() string
	ext    string
	render func(tableData) string
}

var (
	formatMarkdown  = tableFormat{label: locales.TableCopyMarkdown, ext: tableFileMarkdown, render: tableData.markdown}
	formatCSV       = tableFormat{label: locales.TableCopyCsv, ext: tableFileCSV, render: tableData.csv}
	formatTSV       = tableFormat{label: locales.TableCopyTsv, render: tableData.tsv}
	copyFormats     = []tableFormat{formatMarkdown, formatCSV, formatTSV}
	downloadFormats = []tableFormat{formatCSV, formatMarkdown}
)

type tableModal struct {
	open bool
	data tableData
}

type columnWidths struct {
	preferred, least       []float32
	sumPreferred, sumLeast float32
}

type tableLayout struct {
	src    string
	cols   columnWidths
	width  float32
	fitted []float32
}

type layoutKey struct{}

type cellPiece struct {
	dest  string
	link  bool
	spans []ui.Span
}

type inlineStyle struct {
	weight              int
	italic, strike      bool
	code, muted, header bool
}

func (v *View) block(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	if b.Kind != parser.Table {
		return false
	}
	tk := v.tk
	limit := v.rowLimit
	src := d.Source()[b.Start:b.End]
	data := tableOf(d, i, false)
	box := ui.Column(c).AlignSelf(ui.Stretch).MinWidth(0).Border(tableBorder, tk.Border).Radius(theme.RadiusInset).
		Background(tk.Surface).Clip()
	lay := ui.Local(box, layoutKey{}, func() tableLayout { return tableLayout{} })
	if lay.src != src || lay.fitted == nil {
		*lay = tableLayout{src: src, cols: measureColumns(c, tk, data), width: -1}
	}
	if w := box.LayoutBounds().W - 2*tableBorder; w != lay.width || lay.fitted == nil {
		lay.width, lay.fitted = w, lay.cols.fit(w)
	}
	width, services := lay.width, c.Services()
	box.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W-2*tableBorder != width {
			services.Invalidate()
		}
	})
	fitted := lay.fitted
	box.Children(func() {
		ui.Row(c).MinHeight(tableToolbarHeight).AlignItems(ui.Center).Justify(ui.End).Gap(tableToolbarGap).
			PaddingX(tableToolbarPadX).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSubtle).Children(func() {
			v.tableTools(c, tk, func() tableData { return data })
			if toolButton(c, tk, "maximize-2", locales.TableFullscreen()).Clicked() {
				v.table = tableModal{open: true, data: tableOf(d, i, true)}
			}
		})
		ui.ScrollHorizontal(c).AlignSelf(ui.Stretch).Children(func() { v.tableGrid(c, tk, data, fitted, limit) })
	})
	return true
}

func (v *View) tableTools(c *ui.Context, tk theme.Tokens, data func() tableData) {
	toolButton(c, tk, "copy", locales.TableCopy()).Menu(func(m *ui.Menu) {
		for _, f := range copyFormats {
			if m.Item(f.label()).Chosen() {
				c.WriteClipboard(f.render(data()))
			}
		}
	})
	toolButton(c, tk, "download", locales.TableDownload()).Menu(func(m *ui.Menu) {
		for _, f := range downloadFormats {
			if m.Item(f.label()).Chosen() {
				v.download(tableFileBase+"."+f.ext, []byte(f.render(data())))
			}
		}
	})
}

func (v *View) tableGrid(c *ui.Context, tk theme.Tokens, t tableData, widths []float32, limit int) ui.Element {
	rows := t.rows
	if limit > 0 && len(rows) > limit+1 {
		rows = rows[:limit+1]
	}
	grid := ui.Column(c).Shrink(0).Role(ui.RoleTable)
	return grid.Children(func() {
		for r, row := range rows {
			header := r == 0
			rowEl := ui.Row(c).Shrink(0).AlignItems(ui.Stretch).Role(ui.RoleRow)
			if header {
				rowEl.Background(tk.SurfaceMuted)
			} else {
				rowEl.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
			}
			rowEl.Children(func() {
				for k := range t.align {
					var in []parser.Inline
					if k < len(row) {
						in = row[k]
					}
					v.tableCell(c, tk, in, header, t.align[k], widths[k])
				}
			})
		}
	})
}

func (v *View) tableCell(c *ui.Context, tk theme.Tokens, in []parser.Inline, header bool, align parser.Align, width float32) {
	cell := ui.Column(c).Width(width).Shrink(0).Padding(tableCellPadY, tableCellPadX)
	if header {
		cell.Role(ui.RoleColumnHeader).FontSize(tableHeaderSize).FontWeight(mediumWeight).TextColor(tk.TextSecondary)
	} else {
		cell.Role(ui.RoleCell).FontFeatures("tnum")
	}
	cell.Children(func() {
		text := v.richPieces(c, tk, cellPieces(tk, in, header)).AlignSelf(ui.Stretch).TextAlign(textAlign(align))
		if header {
			text.NoWrap()
		}
	})
}

func textAlign(a parser.Align) ui.Align {
	switch a {
	case parser.AlignCenter:
		return ui.Center
	case parser.AlignRight:
		return ui.End
	}
	return ui.Start
}

func (v *View) richPieces(c *ui.Context, tk theme.Tokens, pieces []cellPiece) ui.Element {
	if len(pieces) == 0 {
		return ui.Text(c, "")
	}
	rest := pieces
	var el ui.Element
	if pieces[0].link {
		el = ui.RichText(c)
	} else {
		el = ui.RichText(c, pieces[0].spans...)
		rest = pieces[1:]
	}
	if len(rest) == 0 {
		return el
	}
	return el.Children(func() {
		for _, p := range rest {
			if !p.link {
				ui.RichText(c, p.spans...)
				continue
			}
			l := ui.Link(c, "", "").TextColor(linkColor(tk)).Tooltip(p.dest)
			l.Children(func() { ui.RichText(c, p.spans...) })
			if l.Clicked() {
				v.showLink(p.dest)
			}
		}
	})
}

func cellPieces(tk theme.Tokens, in []parser.Inline, header bool) []cellPiece {
	b := pieceBuilder{tk: tk, size: bodySize}
	if header {
		b.size = tableHeaderSize
	}
	b.inlines(in, 0, len(in), inlineStyle{header: header}, "", false)
	return b.pieces
}

type pieceBuilder struct {
	tk     theme.Tokens
	size   float32
	pieces []cellPiece
}

func (b *pieceBuilder) inlines(in []parser.Inline, from, to int, s inlineStyle, dest string, link bool) {
	for k := from; k < to; k = in[k].Next {
		n := &in[k]
		cs := s
		switch n.Kind {
		case parser.Text, parser.HTML:
			b.add(n.Text, s, dest, link)
		case parser.SoftBreak:
			b.add(" ", s, dest, link)
		case parser.LineBreak:
			b.add("\n", s, dest, link)
		case parser.Code, parser.Math:
			cs.code = true
			b.add(n.Text, cs, dest, link)
		case parser.Emphasis:
			cs.italic = true
			b.inlines(in, k+1, n.Next, cs, dest, link)
		case parser.Strong:
			cs.weight = mediumWeight
			b.inlines(in, k+1, n.Next, cs, dest, link)
		case parser.Strikethrough:
			cs.strike = true
			b.inlines(in, k+1, n.Next, cs, dest, link)
		case parser.Link:
			to, inLink := dest, link
			if !inLink {
				to, inLink = n.Dest, true
			}
			b.inlines(in, k+1, n.Next, cs, to, inLink)
		case parser.Image:
			cs.muted = true
			b.add("["+parser.PlainText(in[k+1:n.Next])+"]", cs, dest, link)
		case parser.FootnoteRef:
			b.add("["+n.Text+"]", s, dest, link)
		}
	}
}

func (b *pieceBuilder) add(text string, s inlineStyle, dest string, link bool) {
	if text == "" {
		return
	}
	sp := ui.Span{Text: text, Size: b.size, Weight: s.weight, Italic: s.italic, Strikethrough: s.strike}
	if s.header {
		sp.Weight = max(sp.Weight, mediumWeight)
	}
	if s.code {
		sp.Font, sp.Size = theme.FontMono, b.size*codeSizeEm
		sp.Background, sp.Radius = b.tk.SurfaceMuted, inlineCodeRadius
	}
	if s.muted {
		sp.Color = b.tk.TextMuted
	}
	if n := len(b.pieces); n == 0 || b.pieces[n-1].link != link || b.pieces[n-1].dest != dest {
		b.pieces = append(b.pieces, cellPiece{dest: dest, link: link})
	}
	last := &b.pieces[len(b.pieces)-1]
	last.spans = append(last.spans, sp)
}

func measureColumns(c *ui.Context, tk theme.Tokens, t tableData) columnWidths {
	cols := len(t.align)
	w := columnWidths{preferred: make([]float32, cols), least: make([]float32, cols)}
	for r, row := range t.rows {
		for k := 0; k < cols && k < len(row); k++ {
			var spans []ui.Span
			for _, p := range cellPieces(tk, row[k], r == 0) {
				spans = append(spans, p.spans...)
			}
			full, _ := c.MeasureText(0, spans...)
			least := full
			if r > 0 {
				least = min(longestWord(c, spans), tableWordCap)
			}
			w.preferred[k] = max(w.preferred[k], full+2*tableCellPadX)
			w.least[k] = max(w.least[k], least+2*tableCellPadX)
		}
	}
	for k := range cols {
		w.least[k] = max(w.least[k], 2*tableCellPadX)
		w.preferred[k] = max(w.preferred[k], w.least[k])
		w.sumPreferred += w.preferred[k]
		w.sumLeast += w.least[k]
	}
	return w
}

func (w columnWidths) fit(width float32) []float32 {
	out := make([]float32, len(w.preferred))
	switch {
	case width <= 0:
		copy(out, w.preferred)
	case w.sumLeast >= width:
		copy(out, w.least)
	case w.sumPreferred <= width:
		for k := range out {
			out[k] = w.preferred[k] * width / w.sumPreferred
		}
	default:
		share := (width - w.sumLeast) / (w.sumPreferred - w.sumLeast)
		for k := range out {
			out[k] = w.least[k] + (w.preferred[k]-w.least[k])*share
		}
	}
	return out
}

func longestWord(c *ui.Context, spans []ui.Span) float32 {
	var widest float32
	for _, sp := range spans {
		for _, word := range words(sp.Text) {
			piece := sp
			piece.Text = word
			w, _ := c.MeasureText(0, piece)
			widest = max(widest, w)
		}
	}
	return widest
}

func words(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		switch {
		case unicode.IsSpace(r):
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			if start >= 0 {
				out = append(out, s[start:i])
			}
			out = append(out, string(r))
			start = -1
		case start < 0:
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

func tableOf(d *parser.Document, i int, keep bool) tableData {
	t := tableData{align: d.Block(i).Align}
	if keep {
		t.align = slices.Clone(t.align)
	}
	for r := range d.Children(i) {
		var cells [][]parser.Inline
		for cell := range d.Children(r) {
			in := d.Inlines(cell)
			if keep {
				in = slices.Clone(in)
			}
			cells = append(cells, in)
		}
		t.rows = append(t.rows, cells)
	}
	return t
}

func parseTable(src string) tableData {
	d := parser.New(parser.GFM)
	d.Set(src)
	for i := 1; i < d.Len(); i++ {
		if d.Block(i).Kind == parser.Table {
			return tableOf(d, i, true)
		}
	}
	return tableData{}
}

func (t tableData) texts() [][]string {
	out := make([][]string, len(t.rows))
	for r, row := range t.rows {
		out[r] = make([]string, len(t.align))
		for k := range t.align {
			if k < len(row) {
				out[r][k] = strings.TrimSpace(parser.PlainText(row[k]))
			}
		}
	}
	return out
}

func (t tableData) csv() string {
	return joinRows(t.texts(), ",", csvCell)
}

func (t tableData) tsv() string {
	return joinRows(t.texts(), "\t", tsvCell)
}

func (t tableData) markdown() string {
	rows := t.texts()
	if len(rows) == 0 {
		return ""
	}
	rule := make([]string, len(t.align))
	for k := range rule {
		rule[k] = "---"
	}
	lines := []string{markdownRow(rows[0], markdownCell), markdownRow(rule, nil)}
	for _, row := range rows[1:] {
		lines = append(lines, markdownRow(row, markdownCell))
	}
	return strings.Join(lines, "\n")
}

func markdownRow(cells []string, escape func(string) string) string {
	if escape != nil {
		cells = mapCells(cells, escape)
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func joinRows(rows [][]string, sep string, escape func(string) string) string {
	lines := make([]string, len(rows))
	for r, row := range rows {
		lines[r] = strings.Join(mapCells(row, escape), sep)
	}
	return strings.Join(lines, "\n")
}

func mapCells(cells []string, fn func(string) string) []string {
	out := make([]string, len(cells))
	for k, cell := range cells {
		out[k] = fn(cell)
	}
	return out
}

func csvCell(s string) string {
	switch {
	case strings.Contains(s, `"`):
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	case strings.ContainsAny(s, ",\n"):
		return `"` + s + `"`
	}
	return s
}

var tsvEscapes = strings.NewReplacer("\t", `\t`, "\n", `\n`, "\r", `\r`)

func tsvCell(s string) string {
	return tsvEscapes.Replace(s)
}

var markdownEscapes = strings.NewReplacer(`\`, `\\`, "|", `\|`)

func markdownCell(s string) string {
	return markdownEscapes.Replace(s)
}

func (v *View) tableDialog(c *ui.Context, tk theme.Tokens) {
	tm := &v.table
	if !tm.open {
		return
	}
	w, h := c.Size()
	ui.DialogBase(c, &tm.open, func(backdrop, panel ui.Element) {
		backdrop.Background(tk.Background)
		fade := panel.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: fullscreenFade, Value: 1, Ease: easeEnter}})
		panel.Size(w, h).Background(tk.Background).TextColor(tk.Foreground).FontSize(bodySize).
			LineHeight(bodyLineHeight).Label(locales.TableFullscreen()).Opacity(fade)
		ui.Row(c).Shrink(0).Justify(ui.End).AlignItems(ui.Center).Gap(tableToolbarGap).Padding(fullscreenPadding).Children(func() {
			data := tm.data
			v.tableTools(c, tk, func() tableData { return data })
			if toolButton(c, tk, closeGlyph, locales.ArcDialogClose()).Clicked() {
				tm.open = false
			}
		})
		widths := measureColumns(c, tk, tm.data).fit(w - 2*fullscreenPadding - 2*tableBorder)
		ui.ScrollBoth(c).Grow(1).MinHeight(0).Padding(0, fullscreenPadding, fullscreenPadding, fullscreenPadding).Children(func() {
			v.tableGrid(c, tk, tm.data, widths, 0).Border(tableBorder, tk.Border).Selectable()
		})
	})
}
