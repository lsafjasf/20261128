package layout

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const epsilon = 1e-6

type missingPos struct {
	r rune
	x float64
}

type measuredText struct {
	width   float64
	missing []missingPos
}

type measuredItem struct {
	item    inlineItem
	width   float64
	missing []missingPos
	ruby    []measuredText
}

type placedItem struct {
	measuredItem
	x float64
}

type wrappedLine struct {
	items []measuredItem
	width float64
}

type layoutContext struct {
	face       MetricFace
	fallback   rune
	fontSize   float64
	rubySize   float64
	lineHeight float64
	rubyLineH  float64
	rubyGap    float64
	style      computedStyle
}

// LayoutTTML parses one TTML document, selects a paragraph, and lays it out.
func LayoutTTML(data []byte, options Options) (*Result, error) {
	doc, err := parseDocument(data)
	if err != nil {
		return nil, err
	}
	options = options.withDefaults()
	viewportW, viewportH := resolveViewport(doc, options)

	paragraph, err := selectParagraph(doc, options.Paragraph)
	if err != nil {
		return nil, err
	}
	region, regStyle, err := selectRegion(doc, paragraph, options, viewportW, viewportH)
	if err != nil {
		return nil, err
	}
	style := resolveParagraphStyle(doc, paragraph, regStyle, viewportW, viewportH)
	if style.lineHeight < 1 {
		style.lineHeight = style.fontSize * 1.2
	}

	items := parseInline(paragraph.Content)
	ctx := &layoutContext{
		face:       options.Face,
		fallback:   options.FallbackRune,
		fontSize:   style.fontSize,
		rubySize:   style.fontSize * options.RubyScale,
		lineHeight: style.lineHeight,
		rubyLineH:  style.fontSize * options.RubyScale * 1.2,
		rubyGap:    options.RubyGap,
		style:      style,
	}
	return layoutItems(items, region, options, ctx)
}

func resolveViewport(doc *document, options Options) (float64, float64) {
	width, height := 1280.0, 720.0
	if options.ViewportWidth > 0 {
		width = options.ViewportWidth
	}
	if options.ViewportHeight > 0 {
		height = options.ViewportHeight
	}
	if options.ViewportWidth <= 0 || options.ViewportHeight <= 0 {
		if raw := doc.rootAttrs["extent"]; raw != "" {
			if extentW, extentH, ok := parsePair(raw, width, height, 32, width, height); ok {
				if options.ViewportWidth <= 0 && extentW > 0 {
					width = extentW
				}
				if options.ViewportHeight <= 0 && extentH > 0 {
					height = extentH
				}
			}
		}
	}
	return width, height
}

func selectParagraph(doc *document, index int) (paragraph, error) {
	if index < 0 || index >= len(doc.paragraphs) {
		return paragraph{}, fmt.Errorf("paragraph index %d out of range [0,%d)", index, len(doc.paragraphs))
	}
	return doc.paragraphs[index], nil
}

func selectRegion(doc *document, p paragraph, options Options, viewportW, viewportH float64) (Rect, map[string]string, error) {
	regionID := options.RegionID
	if regionID == "" {
		regionID = p.Region
	}
	if regionID == "" && len(doc.regionOrder) == 1 {
		regionID = doc.regionOrder[0]
	}
	props := cloneProps(doc.rootAttrs)
	doc.mergeStyle(props, doc.rootStyle, make(map[string]bool))

	spec, exists := doc.regions[regionID]
	if regionID != "" && !exists {
		return Rect{}, nil, fmt.Errorf("region %q not found", regionID)
	}
	if exists {
		doc.mergeStyle(props, spec.Style, make(map[string]bool))
		for key, value := range spec.Attrs {
			props[key] = value
		}
	}
	rect := Rect{Width: viewportW, Height: viewportH}
	if exists {
		var origin, extent string
		if spec.Origin != "" {
			origin = spec.Origin
		} else {
			origin = props["origin"]
		}
		if spec.Extent != "" {
			extent = spec.Extent
		} else {
			extent = props["extent"]
		}
		rect = rectFromTTML(origin, extent, viewportW, viewportH, 32)
		if rect.Width <= 0 {
			rect.Width = viewportW - rect.X
		}
		if rect.Height <= 0 {
			rect.Height = viewportH - rect.Y
		}
	}
	return rect, props, nil
}

func resolveParagraphStyle(doc *document, p paragraph, regionProps map[string]string, viewportW, viewportH float64) computedStyle {
	props := cloneProps(regionProps)
	doc.mergeStyle(props, p.Style, make(map[string]bool))
	for key, value := range p.Attrs {
		props[key] = value
	}
	rootFont := 32.0
	if raw := doc.rootAttrs["fontSize"]; raw != "" {
		if size, ok := parseLength(raw, rootFont, rootFont, viewportW, viewportH); ok {
			rootFont = size
		}
	}
	regionStyle := computeStyle(regionProps, rootFont, viewportW, viewportH)
	return computeStyle(props, regionStyle.fontSize, viewportW, viewportH)
}

func cloneProps(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (ctx *layoutContext) measureText(text string, size float64) measuredText {
	var result measuredText
	for _, r := range text {
		em, ok := ctx.face.Advance(r)
		width := em * size
		if !ok {
			if isExtender(r) {
				continue
			}
			replacement, replacementOK := ctx.face.Advance(ctx.fallback)
			if replacementOK {
				width = replacement * size
			} else {
				width = size
			}
			result.missing = append(result.missing, missingPos{r: r, x: result.width})
		}
		if width < 0 || math.IsNaN(width) || math.IsInf(width, 0) {
			width = 0
		}
		result.width += width
	}
	result.width = finite(result.width)
	return result
}

func (ctx *layoutContext) measureItem(item inlineItem) measuredItem {
	result := measuredItem{item: item}
	result.missing = nil
	if item.kind == itemBreak {
		return result
	}
	base := ctx.measureText(item.text, ctx.fontSize)
	result.width = base.width
	result.missing = base.missing
	result.ruby = make([]measuredText, len(item.layers))
	for i, layer := range item.layers {
		result.ruby[i] = ctx.measureText(layer, ctx.rubySize)
	}
	return result
}

func (ctx *layoutContext) wrap(items []inlineItem, maxWidth float64) []wrappedLine {
	if maxWidth <= 0 {
		maxWidth = 1
	}
	var lines []wrappedLine
	var current []measuredItem
	currentWidth := 0.0

	flush := func(force bool) {
		for len(current) > 0 && current[len(current)-1].item.kind == itemText && current[len(current)-1].item.text == " " {
			current = current[:len(current)-1]
		}
		if len(current) == 0 {
			if force {
				lines = append(lines, wrappedLine{})
			}
			currentWidth = 0
			return
		}
		width := 0.0
		for _, item := range current {
			width += item.width
		}
		lines = append(lines, wrappedLine{items: current, width: width})
		current = nil
		currentWidth = 0
	}

	var process func(inlineItem)
	process = func(item inlineItem) {
		if item.kind == itemBreak {
			flush(false)
			return
		}
		if item.kind == itemText && item.text == " " {
			if len(current) > 0 {
				space := ctx.measureItem(item)
				current = append(current, space)
				currentWidth += space.width
			}
			return
		}
		measured := ctx.measureItem(item)
		if len(current) > 0 && currentWidth+measured.width > maxWidth+epsilon {
			if noBreakBefore(item.text) {
				lastIndex := len(current) - 1
				for lastIndex > 0 && current[lastIndex].item.kind == itemText && current[lastIndex].item.text == " " {
					lastIndex--
				}
				if lastIndex > 0 {
					carry := current[lastIndex]
					current = append(current[:lastIndex], current[lastIndex+1:]...)
					currentWidth = 0
					for _, existing := range current {
						currentWidth += existing.width
					}
					flush(false)
					process(carry.item)
					process(item)
					return
				}
			}
			if !noBreakBefore(item.text) {
				flush(false)
			}
		}
		if len(current) == 0 && measured.width > maxWidth+epsilon {
			parts := ctx.explodeForWidth(item)
			if len(parts) > 1 {
				for _, part := range parts {
					process(part)
				}
				return
			}
		}
		current = append(current, measured)
		currentWidth += measured.width
	}

	for _, item := range items {
		process(item)
	}
	flush(false)
	return lines
}

func (ctx *layoutContext) explodeForWidth(item inlineItem) []inlineItem {
	if item.kind == itemRuby {
		return ctx.explodeRubyByWidth(item)
	}
	if item.kind != itemText {
		return []inlineItem{item}
	}
	clusters := graphemes(item.text)
	if len(clusters) <= 1 {
		return []inlineItem{item}
	}
	result := make([]inlineItem, len(clusters))
	for i, cluster := range clusters {
		result[i] = inlineItem{kind: itemText, text: cluster}
	}
	return result
}

func (ctx *layoutContext) explodeRubyByWidth(item inlineItem) []inlineItem {
	clusters := graphemes(item.text)
	if len(clusters) <= 1 {
		return []inlineItem{item}
	}
	result := make([]inlineItem, len(clusters))
	weights := make([]float64, len(clusters))
	for i, cluster := range clusters {
		result[i] = inlineItem{kind: itemText, text: cluster}
		weights[i] = ctx.measureText(cluster, ctx.fontSize).width
	}
	for layerIndex, layer := range item.layers {
		pieces := distributeClusters(layer, weights)
		for i, piece := range pieces {
			if len(result[i].layers) <= layerIndex {
				result[i].layers = append(result[i].layers, make([]string, layerIndex-len(result[i].layers)+1)...)
			}
			result[i].layers[layerIndex] = piece
			if piece != "" {
				result[i].kind = itemRuby
			}
		}
	}
	return result
}

func noBreakBefore(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	switch r {
	case '、', '。', '，', '．', '！', '？', '：', '；', '）', '］', '｝',
		'」', '』', '】', '〉', '》', '〕', '〗', '〙', '〛', '・', '…', '—':
		return true
	default:
		return false
	}
}

func layoutItems(items []inlineItem, region Rect, options Options, ctx *layoutContext) (*Result, error) {
	padTop := options.TopPadding + ctx.style.padding[0]
	padRight := options.RightPadding + ctx.style.padding[1]
	padBottom := options.BottomPadding + ctx.style.padding[2]
	padLeft := options.LeftPadding + ctx.style.padding[3]
	inner := Rect{
		X:      region.X + padLeft,
		Y:      region.Y + padTop,
		Width:  region.Width - padLeft - padRight,
		Height: region.Height - padTop - padBottom,
	}
	if inner.Width <= 0 || inner.Height <= 0 {
		return nil, fmt.Errorf("region %.2fx%.2f has no content box after padding", region.Width, region.Height)
	}

	lines := ctx.wrap(items, inner.Width)
	result := &Result{Region: region}
	bodyHeight := float64(len(lines)) * ctx.lineHeight
	bodyTop := inner.Y
	switch strings.TrimSpace(ctx.style.displayAlign) {
	case "before", "top":
		bodyTop = inner.Y
	case "after", "bottom":
		bodyTop = inner.Bottom() - bodyHeight
	default:
		bodyTop = inner.Y + (inner.Height-bodyHeight)/2
	}

	ascent := ctx.face.Ascent() * ctx.fontSize
	descent := ctx.face.Descent() * ctx.fontSize
	leading := ctx.lineHeight - ascent - descent
	if leading < 0 {
		leading = 0
	}
	for lineIndex, wrapped := range lines {
		lineY := bodyTop + float64(lineIndex)*ctx.lineHeight
		lineX := alignLineX(ctx.style.textAlign, inner, wrapped.width)
		line := Line{
			Box:      Rect{X: lineX, Y: lineY, Width: wrapped.width, Height: ctx.lineHeight},
			Baseline: lineY + leading/2 + ascent,
		}
		itemX := lineX
		var text strings.Builder
		placed := make([]placedItem, 0, len(wrapped.items))
		for _, item := range wrapped.items {
			text.WriteString(item.item.text)
			placed = append(placed, placedItem{measuredItem: item, x: itemX})
			itemX += item.width
		}
		line.Text = text.String()
		for _, item := range placed {
			for _, missing := range item.missing {
				entry := makeMissing(missing.r, "base", lineIndex+1, item.x+missing.x)
				line.Missing = append(line.Missing, entry)
				result.Missing = append(result.Missing, entry)
			}
			for layerIndex, layer := range item.ruby {
				if layerIndex >= len(item.item.layers) || item.item.layers[layerIndex] == "" {
					continue
				}
				rubyLine, missing := ctx.rubyLine(item, layerIndex, layer, lineIndex, lineY)
				line.Ruby = append(line.Ruby, rubyLine)
				line.Missing = append(line.Missing, missing...)
				result.Missing = append(result.Missing, missing...)
			}
		}
		result.Lines = append(result.Lines, line)
		result.ContentBounds = result.ContentBounds.Union(line.Box)
		for _, rubyLine := range line.Ruby {
			result.ContentBounds = result.ContentBounds.Union(rubyLine.Box)
		}
	}
	return result, nil
}

func (ctx *layoutContext) rubyLine(item placedItem, layerIndex int, layer measuredText, lineIndex int, lineY float64) (RubyLine, []MissingGlyph) {
	rubyX := alignedRubyX(ctx.style.rubyAlign, item.x, item.width, layer.width)
	var top float64
	position := strings.TrimSpace(ctx.style.rubyPosition)
	if position == "after" {
		top = lineY + ctx.lineHeight + ctx.rubyGap + float64(layerIndex)*ctx.rubyLineH
	} else {
		top = lineY - ctx.rubyGap - float64(layerIndex+1)*ctx.rubyLineH
	}
	rubyAscent := ctx.face.Ascent() * ctx.rubySize
	rubyDescent := ctx.face.Descent() * ctx.rubySize
	leading := ctx.rubyLineH - rubyAscent - rubyDescent
	if leading < 0 {
		leading = 0
	}
	ruby := RubyLine{
		Layer:    layerIndex,
		Text:     item.item.layers[layerIndex],
		Box:      Rect{X: rubyX, Y: top, Width: layer.width, Height: ctx.rubyLineH},
		Baseline: top + leading/2 + rubyAscent,
	}
	missing := make([]MissingGlyph, 0, len(layer.missing))
	for _, entry := range layer.missing {
		glyph := makeMissing(entry.r, fmt.Sprintf("ruby[%d]", layerIndex), lineIndex+1, rubyX+entry.x)
		missing = append(missing, glyph)
	}
	ruby.Missing = missing
	return ruby, missing
}

func alignLineX(align string, inner Rect, width float64) float64 {
	switch strings.TrimSpace(align) {
	case "center", "middle":
		return inner.X + (inner.Width-width)/2
	case "end", "right":
		return inner.Right() - width
	default:
		return inner.X
	}
}

func alignedRubyX(align string, itemX, itemWidth, rubyWidth float64) float64 {
	switch strings.TrimSpace(align) {
	case "start", "left":
		return itemX
	case "end", "right":
		return itemX + itemWidth - rubyWidth
	default:
		return itemX + (itemWidth-rubyWidth)/2
	}
}

func makeMissing(r rune, layer string, line int, x float64) MissingGlyph {
	return MissingGlyph{
		Rune:    string(r),
		Code:    fmt.Sprintf("U+%04X", r),
		Layer:   layer,
		Line:    line,
		X:       finite(x),
		Message: "no glyph in fallback chain; replacement glyph used",
	}
}
